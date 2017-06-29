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
	"log"
	"sync"

	"github.com/cenkalti/backoff"
	influx "github.com/influxdata/influxdb/client/v2"
)

type bufferedSampledMetricsPush struct {
	influxDBName    string
	influxDbClient  influx.Client
	influxBatchSize int

	donec chan struct{}
	bufMu sync.Mutex
	buf   []*timingInfo
}

type timingInfo struct {
	Region       string `json:"name"`
	CountryCode  string `json:"country"`
	Query        string `json:"query"`
	Type         string `json:"qtype"`
	RecordsCount int    `json:"reccount"`
	Latency      int    `json:"latency"`
}

func (b *bufferedSampledMetricsPush) run(ch <-chan *timingInfo) {
	debugLog.Printf("Starting buffered batched metrics push to influx, batch size %v", b.influxBatchSize)
	b.donec = make(chan struct{})
	b.buf = make([]*timingInfo, 0, b.influxBatchSize+100)
	go func() {
		for m := range ch {
			if n := b.appendBuf(m); n >= b.influxBatchSize {
				b.flush()
			}
		}
		b.flush()
		close(b.donec)
	}()
}

func (b *bufferedSampledMetricsPush) done() <-chan struct{} {
	return b.donec
}

func (b *bufferedSampledMetricsPush) appendBuf(m *timingInfo) int {
	b.bufMu.Lock()
	defer b.bufMu.Unlock()
	b.buf = append(b.buf, m)
	return len(b.buf)
}

func (b *bufferedSampledMetricsPush) flush() {
	b.bufMu.Lock()
	defer b.bufMu.Unlock()
	n := len(b.buf)
	for n > 0 {
		if n > b.influxBatchSize {
			n = b.influxBatchSize
		}
		buf := make([]*timingInfo, n)
		copy(buf, b.buf[:n])
		// Doing this in a go routine to not block the buffer. If it fails we
		// don't care since it's "just" metrics
		go b.sendToInflux(buf)
		b.buf = b.buf[n:]
		n = len(b.buf)
	}
}

func (b *bufferedSampledMetricsPush) sendToInflux(tis []*timingInfo) {
	// Then write the info to the influx db
	// Create a new point batch
	bp, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Database:  b.influxDBName,
		Precision: "ns",
	})
	if err != nil {
		log.Printf("Couldn't create influx batch points config: %s", err)
	}

	for _, ti := range tis {
		tags := map[string]string{"region": ti.Region, "country": ti.CountryCode}
		fields := map[string]interface{}{
			"avglatency":   ti.Latency,
			"tickets-sold": ti.RecordsCount,
		}
		pt, err := influx.NewPoint("ticket_sale", tags, fields)
		if err != nil {
			log.Printf("Couldn't create influx point: %s", err)
		}
		bp.AddPoint(pt)

	}
	// Write the batch
	err = backoff.Retry(func() error {
		return b.influxDbClient.Write(bp)
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 10))
	if err != nil {
		log.Printf("Couldn't write to influxDB %s", err)
	}
}
