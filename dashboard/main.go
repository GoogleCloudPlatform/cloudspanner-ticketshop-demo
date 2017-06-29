/*
Copyright 2018 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/namsral/flag"
	"golang.org/x/net/websocket"
	validator "gopkg.in/go-playground/validator.v9"
)

// debugging log
type debugging bool

func (d debugging) Printf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}

type config struct {
	Debug            bool
	SSL              bool
	Port             int
	InfluxAddr       string `validate:"required"`
	InfluxDBName     string `validate:"required"`
	InfluxDBUsername string
	InfluxDBPassword string
	MetricRefreshMS  int
	Countries        string
	Regions          string
}

var (
	clients  []*wsClient
	debugLog debugging
	version  string // set by linker -X
	build    string // set by linker -X

)

func processFlags(c *config) error {
	flag.String(flag.DefaultConfigFlagname, "", "path to config file")
	flag.BoolVar(&c.Debug, "debug", false, "Enable debug output to stdout")
	flag.BoolVar(&c.SSL, "ssl", true, "For local dev start server set to false")
	flag.IntVar(&c.Port, "port", 8080, "HTTP/HTTPS Server port")
	flag.StringVar(&c.InfluxAddr, "influx_addr", "http://localhost:8086",
		"The Addr connection string to your influx db")
	flag.StringVar(&c.InfluxDBName, "influx_database", "",
		"The name of your influx db")
	flag.StringVar(&c.InfluxDBUsername, "influx_username", "",
		"The username for your influx db")
	flag.StringVar(&c.InfluxDBPassword, "influx_password", "",
		"The password for your influx db")
	flag.IntVar(&c.MetricRefreshMS, "refresh_cycle_ms", 2000,
		"Metrics refresh cycle in ms")
	flag.StringVar(&c.Countries, "countries", "DE,TW,US",
		"Comma-separated list of countries to watch")
	flag.StringVar(&c.Regions, "regions", "EU,AS,NA",
		"Comma-separated list of regions to watch")
	flag.Parse()

	debugLog = debugging(c.Debug)

	debugLog.Printf("- Debugging enabled - \n")
	debugLog.Printf("Running dashboard version %v\n", version)
	debugLog.Printf("-- Configuration --\n")
	s := reflect.ValueOf(c).Elem()
	for i := 0; i < s.NumField(); i++ {
		n := s.Type().Field(i).Name
		f := s.Field(i)
		debugLog.Printf("%v: %v\n", n, f.Interface())
	}
	v := validator.New()
	return v.Struct(c)
}

func runcollector(f func(w *window) ([]*metric, error), ch chan<- []*metric, c *config) {
	w := &window{}
	for {
		tw := &window{lower: w.upper, upper: time.Now().UnixNano(), cache: w.cache}
		ms, err := f(tw)
		if err != nil {
			log.Print(err)
		}
		if ms != nil && len(ms) > 0 {
			// if call was successful assign temp window to outer window
			w = tw
			ch <- ms
		}
		d := time.Duration(c.MetricRefreshMS + rand.Intn(c.MetricRefreshMS/10))
		time.Sleep(d * time.Millisecond)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("version:    ", version)
		fmt.Println("build:      ", build)
		os.Exit(0)
	}

	// Initialize rand
	rand.Seed(time.Now().UnixNano())

	config := &config{}
	err := processFlags(config)
	if err != nil {
		fmt.Printf("%+v", err)
		return
	}

	// Initialize Metrics related stuff
	debugLog.Printf("setting up influx with tcp connection.")

	ic := influx.HTTPConfig{
		Addr:     config.InfluxAddr,
		Username: config.InfluxDBUsername,
		Password: config.InfluxDBPassword,
	}
	idbc, err := influx.NewHTTPClient(ic)
	if err != nil {
		log.Printf("Error setting up influx in udp: %s", err)
	}

	var chMetrics = make(chan []*metric, chBufSize)

	icc := influxConn{
		idbc,
		config.InfluxDBName,
		strings.Split(config.Countries, ","),
		strings.Split(config.Regions, ","),
	}

	// get statistics from InfluxDB and push via websocket to all active clients
	go runcollector(icc.regioncountryglobalSold, chMetrics, config)
	time.Sleep(10 * time.Millisecond)
	go runcollector(icc.globalMetrics, chMetrics, config)
	time.Sleep(10 * time.Millisecond)
	go runcollector(icc.regionMetrics, chMetrics, config)

	// websocket clients management
	clients = make([]*wsClient, 0)
	var msh = &wsStreamHandler{
		chAddClient: make(chan *wsClient, chBufSize),
		chDelClient: make(chan *wsClient, chBufSize),
		chMsgs:      chMetrics,
	}

	go msh.watchClients()
	http.Handle("/", http.FileServer(http.Dir("static")))
	http.Handle("/third_party/", http.FileServer(http.Dir(".")))
	http.Handle("/ws", websocket.Handler(msh.handler))

	// TODO: consider using https://golang.org/x/crypto/acme/autocert
	debugLog.Printf("Serving on port %v", config.Port)
	if !config.SSL {
		err = http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil)
	} else {
		err = http.ListenAndServeTLS(
			fmt.Sprintf(":%d", config.Port),
			"certs/server.cert",
			"certs/server.key",
			nil,
		)
	}

	log.Fatal(err)

}
