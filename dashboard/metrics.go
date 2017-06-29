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
	"encoding/json"
	"fmt"

	influx "github.com/influxdata/influxdb/client/v2"
)

type metric struct {
	Name    string `json:"name"`
	Value   int64  `json:"value"`
	Region  string `json:"region,omitempty"`
	Country string `json:"country,omitempty"`
}

func (m *metric) String() string {
	return fmt.Sprintf("%+v", *m)
}

type influxConn struct {
	influx.Client
	dbName    string
	countries []string
	regions   []string
}

type window struct {
	lower int64
	upper int64
	cache interface{}
}

func (ic *influxConn) runQuery(q string, f func(r []influx.Result) ([]*metric, error)) ([]*metric, error) {
	iq := influx.Query{
		Command:  q,
		Database: ic.dbName,
	}

	r, err := ic.Query(iq)
	if err != nil {
		return nil, err
	}

	if r.Error() != nil {
		return nil, r.Error()
	}

	if len(r.Results) == 0 {
		return nil, nil
	}

	return f(r.Results)
}

func (ic *influxConn) globalMetrics(w *window) ([]*metric, error) {
	q := `SELECT percentile(avglatency, 50) as p50, 
			 percentile(avglatency, 90) as p90, 
			 percentile(avglatency, 99) as p99, 
		 	 sum("tickets-sold") AS tickets 
		 FROM ticket_sale 
		 WHERE time > now() - 60s`

	f := func(r []influx.Result) ([]*metric, error) {
		m := make([]*metric, 0, 30)
		for _, s := range r[0].Series {
			if len(s.Values) == 0 || len(s.Values[0]) < 5 {
				return nil, nil
			}
			for k := 1; k < 4; k++ {
				val, err := s.Values[0][k].(json.Number).Int64()
				if err != nil {
					return nil, err
				}
				m = append(m, &metric{
					Name:  "latency_" + s.Columns[k] + "_ms",
					Value: val,
				})
			}
			val, err := s.Values[0][4].(json.Number).Int64()
			if err != nil {
				return nil, err
			}
			m = append(m, &metric{
				Name:  "tickets_sold_per_minute",
				Value: val,
			})
		}

		return m, nil
	}
	return ic.runQuery(q, f)
}

func (ic *influxConn) regionMetrics(w *window) ([]*metric, error) {
	q := `SELECT percentile(avglatency, 50) as p50, 
			percentile(avglatency, 90) as p90, 
			percentile(avglatency, 99) as p99, 
			sum("tickets-sold") as tickets 
		  FROM ticket_sale WHERE time > now() - 60s 
		  GROUP BY region`

	f := func(r []influx.Result) ([]*metric, error) {
		m := make([]*metric, 0, 30)
		for _, s := range r[0].Series {
			if len(s.Values) == 0 || len(s.Values[0]) < 5 {
				return nil, nil
			}
			r := s.Tags["region"]
			for k := 1; k < 4; k++ {
				val, err := s.Values[0][k].(json.Number).Int64()
				if err != nil {
					return nil, err
				}
				m = append(m, &metric{
					Region: r,
					Name:   "latency_" + s.Columns[k] + "_ms",
					Value:  val,
				})
			}
			val, err := s.Values[0][4].(json.Number).Int64()
			if err != nil {
				return nil, err
			}
			m = append(m, &metric{
				Region: r,
				Name:   "tickets_sold_per_minute",
				Value:  val,
			})
		}

		return m, nil
	}

	return ic.runQuery(q, f)
}

func (ic *influxConn) regioncountryglobalSold(w *window) ([]*metric, error) {
	q := fmt.Sprintf(
		`SELECT sum("tickets-sold") 
	     FROM ticket_sale 
		 WHERE time >= %v AND time < %v 
		 GROUP BY region,country`,
		w.lower, w.upper)

	f := func(r []influx.Result) ([]*metric, error) {
		if w.cache == nil {
			// region/country/value
			w.cache = map[string]map[string]int64{}
		}
		rcc := w.cache.(map[string]map[string]int64)

		for _, s := range r[0].Series {
			if len(s.Values) == 0 || len(s.Values[0]) < 2 {
				return nil, nil
			}
			c := s.Tags["country"]
			r := s.Tags["region"]
			v, err := s.Values[0][1].(json.Number).Int64()
			if err != nil {
				return nil, err
			}

			if _, ok := rcc[r]; !ok {
				rcc[r] = map[string]int64{}
			}

			if _, ok := rcc[r][c]; !ok {
				rcc[r][c] = int64(0)
			}
			rcc[r][c] += v
		}

		m := make([]*metric, 0, 10)
		gv := int64(0)
		for r, cc := range rcc {
			rv := int64(0)
			for c, v := range cc {
				m = append(m, &metric{
					Country: c,
					Name:    "total_tickets_sold",
					Value:   v,
				})
				rv = rv + v
			}
			m = append(m, &metric{
				Region: r,
				Name:   "total_tickets_sold",
				Value:  rv,
			})
			gv = gv + rv
		}
		m = append(m, &metric{
			Name:  "total_tickets_sold",
			Value: gv,
		})

		return m, nil
	}

	return ic.runQuery(q, f)
}
