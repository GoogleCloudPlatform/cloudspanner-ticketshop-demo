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
	"log"
	"math/rand"
	"os"
	"reflect"
	"time"

	drand "github.com/dgryski/go-discreterand"
)

// Config store global configuration values.
type Config struct {
	TimeStart       time.Time
	TimeEnd         time.Time
	allForSaleNow   bool
	VenueConfig     map[string]VenueConfig    `json:"venueConfig"`
	MultiEventTypes map[string]MultiEventType `json:"multiEventTypes"`
	weightedDists   map[string]weightedDist
	countries       []Country
}

// VenueConfig stores seating configuration data for a particular venue.
type VenueConfig struct {
	Weight         int                      `json:"weight"`
	NumSeatsRange  []int                    `json:"numSeatsRange"`
	SeatingConfigs map[string]SeatingConfig `json:"seatingConfig"`
}

// SeatingConfig stores seating configuration data for a particular seating
// arrangement in a venue.
type SeatingConfig struct {
	SeatConfigMultiplier int                    `json:"seatConfigMultiplier"`
	Tiers                map[string]SeatingTier `json:"tier"`
}

// SeatingTier stores the distribution weight for a particular seating tier
type SeatingTier struct {
	Weight int `json:"weight"`
}

// MultiEventType stores seating configuration data for a particular multi-event.
type MultiEventType struct {
	Weight             int                         `json:"weight"`
	NumEventsRange     []int                       `json:"numEventsRange"`
	PricingRange       map[string]map[string][]int `json:"pricingRange"`
	VenueSeatingConfig map[string]string           `json:"venueSeatingConfig"`
}

//Country stores a country name and population.
type Country struct {
	Country    string `json:"country"`
	Population int    `json:"population"`
}

// weightedDist stores names and weights for a discrete probability distribution.
type weightedDist struct {
	names []string
	atab  drand.AliasTable
}

// sample generates a random sample from a weighted distribution.
func (w *weightedDist) sample() string {
	sample := w.atab.Next()
	return w.names[sample]
}

func (config *Config) init() error {
	err := config.buildDists()
	if err != nil {
		return err
	}
	err = config.loadPopulationData()
	if err != nil {
		return err
	}
	config.countries = config.cleanCountryData()
	err = config.buildPopulationDist()
	if err != nil {
		return err
	}
	return nil
}

// buildDist builds a weighted probability distribution from config data.
func (config *Config) buildDist(distName string, m interface{}) {
	var total float64
	var weights []int
	var names []string
	var probs []float64

	outerMap := reflect.ValueOf(m)
	for _, k := range outerMap.MapKeys() {
		names = append(names, k.String())
		innerStruct := outerMap.MapIndex(k)
		v := innerStruct.FieldByName("Weight")
		if !v.IsValid() {
			log.Fatalf("couldn't find 'weight' in %v in dist %v ",
				k.String(), distName)
		}
		w := int(v.Int())
		weights = append(weights, w)
		total += float64(w)
	}
	for _, v := range weights {
		probs = append(probs, float64(v)/total)
	}
	wd := weightedDist{names,
		drand.NewAlias(probs, rand.NewSource(time.Now().UnixNano()))}
	if config.weightedDists == nil {
		config.weightedDists = make(map[string]weightedDist)
	}
	config.weightedDists[distName] = wd
}

// buildDists builds weighted probability distributions from config data.
func (config *Config) buildDists() error {
	config.buildDist("venueConfigs", config.VenueConfig)
	for vt, vc := range config.VenueConfig {
		for sct, sc := range vc.SeatingConfigs {
			config.buildDist(vt+"_"+sct, sc.Tiers)
		}
	}
	config.buildDist("multiEventTypes", config.MultiEventTypes)
	return nil
}

// testDist tests a named distribution. TBD: move to config_test.go
func (config *Config) testDist(distName string) {
	freq := make(map[string]int)
	sampler := config.weightedDists[distName]
	for i := 0; i < 100000; i++ {
		s := sampler.sample()
		freq[s]++
	}
	fmt.Printf("results for %s distribution...\n", distName)
	for k, v := range freq {
		fmt.Printf("%v: %v\n", k, v)
	}
}

// sampleDate randomly selects a starting date, given an interval in days,
// which fits  within the global time frame.
func (config *Config) sampleDate(numDays int) (eventDate time.Time, saleDate time.Time, err error) {
	hours := int(config.TimeEnd.Sub(config.TimeStart).Hours())
	randomhrs := rand.Intn(hours)
	eventDate = config.TimeStart.
		Add(time.Duration(randomhrs) * time.Hour).
		Add(time.Duration(15*rand.Intn(4)) * time.Minute)
	if config.allForSaleNow {
		return eventDate, config.TimeStart, nil
	}

	// between yesterday and 60 days before the event
	hours = int(eventDate.Sub(config.TimeStart).Hours()) - 1440
	if hours > 0 {
		randomhrs = rand.Intn(hours)
	} else {
		randomhrs = -24
	}

	saleDate = config.TimeStart.Add(time.Duration(randomhrs) * time.Hour)
	return eventDate, saleDate, nil
}

// displayJSON formats and displays config data for debugging purposes.
func displayJSON(C *Config) {
	s, err := json.MarshalIndent(C, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", s)
}

func (config *Config) loadPopulationData() error {
	f, err := os.Open(*popDataFile)
	if err != nil {
		return err
	}
	err = json.NewDecoder(f).Decode(&config.countries)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func (config *Config) buildPopulationDist() error {
	var total float64
	var weights []int
	var names []string
	var probs []float64
	for _, v := range config.countries {
		names = append(names, v.Country)
		weights = append(weights, v.Population)
		total += float64(v.Population)
	}
	for _, v := range weights {
		probs = append(probs, float64(v)/total)
	}
	wd := weightedDist{names,
		drand.NewAlias(probs, rand.NewSource(time.Now().UnixNano()))}
	if config.weightedDists == nil {
		config.weightedDists = make(map[string]weightedDist)
	}
	config.weightedDists["population"] = wd
	return nil
}

func (config *Config) cleanCountryData() []Country {
	newList := make([]Country, 0, 500)
	for _, v := range config.countries {
		_, err := queryCountries.FindCountryByName(v.Country)
		if err != nil {
			log.Printf("country %v missing from gountries data", v.Country)
		} else {
			newList = append(newList, v)
		}
	}
	return newList
}

// auditConfigData verifies integrity of json config data.
func (config *Config) auditConfigData() error {
	// Assemble map of seating config and tiers per venue type.
	// [venueType][seatingType][tier]
	venueSeatingTypeTiers := make(map[string]map[string][]string)
	for vt, vc := range config.VenueConfig {
		venueSeatingTypeTiers[vt] = make(map[string][]string)
		for sct, sc := range vc.SeatingConfigs {
			for t := range sc.Tiers {
				venueSeatingTypeTiers[vt][sct] = append(venueSeatingTypeTiers[vt][sct], t)
			}
		}
	}

	// Make sure every MultiEventType price range has a
	// corresponding seating config tier config for a given venue type.
	for met, t := range config.MultiEventTypes {
		for vt, prs := range t.PricingRange {
			for pt := range prs {
			vsttLoop:
				for sct, sts := range venueSeatingTypeTiers[vt] {
					// check if pt is in sts
					for _, st := range sts {
						if st == pt {
							continue vsttLoop
						}
					}
					return fmt.Errorf(`
						Missing price range configuration for MultiEventType: %v,
						VenueType: %v, SeatingConfigType: %v, 
						SeatingTier: %v`, met, vt, sct, pt)
				}
			}
		}
	}
	return nil
}
