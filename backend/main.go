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
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"github.com/bluele/gcache"
	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/namsral/flag"
	"github.com/pariz/gountries"
	"gopkg.in/go-playground/validator.v9"
)

type config struct {
	Debug            bool
	APIDoc           bool
	Port             int
	ProjectID        string `validate:"required"`
	InstanceName     string `validate:"required"`
	DBName           string `validate:"required"`
	CacheSize        int
	CacheExpiration  int
	InfluxAddr       string `validate:"required"`
	InfluxDBName     string `validate:"required"`
	InfluxDBUsername string
	InfluxDBPassword string
	InfluxBatchSize  int
}

var (
	version string // set by linker -X
	build   string // set by linker -X

	// influxDbClient influx.Client
	metricsCh chan *timingInfo

	debugLog  debugging
	countries = gountries.New()
)

func processFlags(c *config) error {
	flag.String(flag.DefaultConfigFlagname, "", "path to config file")
	flag.IntVar(&c.Port, "port", 8090, "HTTP Server port")
	flag.StringVar(&c.ProjectID, "project", "", "Your cloud project ID.")
	flag.StringVar(&c.InstanceName, "instance", "",
		"The name of the Spanner instance within your project.")
	flag.StringVar(&c.DBName, "database", "",
		"The name of the database in your Spanner instance.")
	flag.IntVar(&c.CacheSize, "cachesize", 1000,
		"Set cache size (key elements) for db cache.")
	flag.IntVar(&c.CacheExpiration, "cacheexpiration", 15,
		"Sets cache median expiration (seconds) for db cache. Set to 0 to disable.")
	flag.BoolVar(&c.APIDoc, "apidoc", false, "Enable update of api doc")
	flag.BoolVar(&c.Debug, "debug", false, "Enable debug output to stdout")

	flag.StringVar(&c.InfluxAddr, "influx_addr", "http://localhost:8086",
		"The Addr connection string to your influx db")
	flag.StringVar(&c.InfluxDBName, "influx_database", "",
		"The name of your influx db")
	flag.StringVar(&c.InfluxDBUsername, "influx_username", "",
		"The username for your influx db")
	flag.StringVar(&c.InfluxDBPassword, "influx_password", "",
		"The password for your influx db")
	flag.IntVar(&c.InfluxBatchSize, "influx_batchsize", 7,
		"Batch size for write to influxdb")

	flag.Parse()

	debugLog = debugging(c.Debug)

	debugLog.Printf("- Debugging enabled - \n")
	debugLog.Printf("Running ticketshop-backend version %v\n", version)
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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("version:    ", version)
		fmt.Println("build:      ", build)
		os.Exit(0)
	}

	config := &config{}

	err := processFlags(config)
	if err != nil {
		fmt.Printf("%+v", err)
		return
	}

	// Initialize rand
	rand.Seed(time.Now().UnixNano())

	// Initialize Metrics related stuff
	debugLog.Printf("setting up influx with tcp connection.")

	ic := influx.HTTPConfig{
		Addr:     config.InfluxAddr,
		Username: config.InfluxDBUsername,
		Password: config.InfluxDBPassword,
	}
	influxDbClient, err := influx.NewHTTPClient(ic)
	if err != nil {
		log.Printf("Error setting up influx in udp: %s", err)
	}

	bsmp := &bufferedSampledMetricsPush{
		influxDBName:    config.InfluxDBName,
		influxDbClient:  influxDbClient,
		influxBatchSize: config.InfluxBatchSize,
	}
	metricsCh = make(chan *timingInfo, 1e7)
	bsmp.run(metricsCh)

	// Intialize Spanner client and cache
	// ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	// defer cancel()
	// dbc := connect(ctx, dbPath)
	cssvc, err := NewCloudSpannerService(config.ProjectID, config.InstanceName, config.DBName)
	if err != nil {
		log.Fatalf("Couldn't connect to Google Cloud Spanner: %v", err)
	}
	defer cssvc.cleanup()

	cc := make(chan os.Signal, 2)
	signal.Notify(cc, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-cc
		log.Println("Cleaning up resources before exiting!")
		cssvc.cleanup()
		os.Exit(1)
	}()

	var c gcache.Cache
	if config.CacheExpiration > 0 {
		// cache expiration +- 1/4th of the cacheExpiration seconds
		ce := config.CacheExpiration*7/8 + rand.Intn(config.CacheExpiration/4)
		c = gcache.New(config.CacheSize).
			LRU().
			Expiration(time.Duration(ce) * time.Second).
			Build()
		debugLog.Printf("Setting cache expiration to %v sec", ce)
	} else {
		debugLog.Printf("Cache is disabled")
	}

	debugLog.Printf("Serving on port %v", config.Port)
	handler := newEndpointsRouter(cssvc, c, config.CacheExpiration, config.APIDoc)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), handler))
}
