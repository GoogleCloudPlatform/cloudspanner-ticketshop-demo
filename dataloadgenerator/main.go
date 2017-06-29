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
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/spanner"

	"github.com/google/uuid"
	"github.com/namsral/flag"
	"github.com/pariz/gountries"
)

var (
	version        string // set by linker -X
	build          string // set by linker -X
	config         Config
	configFile     = flag.String("configFile", "config.json", "Path to JSON formatted config file. [config.json]")
	popDataFile    = flag.String("popDataFile", "assets/country-by-population.json", "Path to country/population data file. [assets/country-by-population.json]")
	timeout        = flag.Int("timeout", 10, "Timeout value (in seconds) for inserts.")
	project        = flag.String("project", "", "Your cloud project ID.")
	instance       = flag.String("instance", "", "The Spanner instance within your project.")
	dbName         = flag.String("database", "", "The database name in your Spanner instance.")
	infoDataSize   = flag.Int("infoDataSize", 0, "Bytes of random data to add per venue, ticket, or event")
	batchSize      = flag.Int("batchSize", 1500, "Max number of mutations to send in one Spanner request.")
	parallelTxn    = flag.Int("parallelTxn", 10, "Max number of parallel requests to Cloud Spanner.")
	eventDateRange = flag.Int("eventDateRange", 100, "Events date range in days.")
	dontask        = flag.Bool("dontask", false, "Set to true if you don't want to be asked for permission to delete data! e.g. non-interactive mode [false]")
	dryrun         = flag.Bool("dryrun", false, "No data persisted if dryrun set to true")
	allForSaleNow  = flag.Bool("allForSaleNow", true, "Set to false if you want Ticket sale dates between 1 and 60 days before the event")

	dbPath = ""
	// client             *spanner.Client
	venuesDB           = make([]dbVenueLoad, 0, 1e6)
	ticketSeqNums      = make(map[string]int64)
	queryCountries     = gountries.New()
	continents         = []string{"EU", "AS", "AF", "OC", "SA", "AN", "NA"}
	venueNameAdditions = []string{"Fabulous", "Pavillion", "Friendship", "Amazing", "Tropical", "Beach", "Outdoor", "Historic", "New", "Old", "Central"}
	maxSeatsPerCat     = int64(10000)
)

const bytesToGibi = 1024 * 1024 * 1024

type eventStats struct {
	EventType  string `json:"EventType"`
	VenueType  string `json:"VenueType"`
	NumEvents  int    `json:"NumEvents"`
	TotalSeats int    `json:"TotalSeats"`
}

type generatorStats struct {
	NumMultiEvents   int                    `json:"numMultiEvents"`
	NumEvents        int                    `json:"numEvents"`
	NumTickets       int64                  `json:"numTickets"`
	SumTicketPrices  float64                `json:"sumTicketPrices"`
	TotalDiskSpaceGB float64                `json:"totalDiskSpaceGB"`
	EventDistr       map[string]*eventStats `json:"eventDistr"`
}

type multiEventGenDef struct {
	multiEventType string
	events         []*eventGenDef
	totalSeats     int64
}

type eventGenDef struct {
	venue             dbVenue
	venueType         string
	totalSeats        int64
	seatingCategories []*dbSeatingCategory
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("version:    ", version)
		fmt.Println("build:      ", build)
		os.Exit(0)
	}

	validCommands := []string{"create", "reset", "venues", "events"}
	flag.Parse()

	f, err := os.Open(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	err = json.NewDecoder(f).Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
	err = config.auditConfigData()
	if err != nil {
		log.Fatalf("config data failed audit: %v", err)
	}
	// Uncomment next line for debugging purposes.
	//displayJSON(&config)

	if len(flag.Args()) < 1 {
		log.Fatalf("Error: no command specified, choose one of %v\n", validCommands)
	}
	if *project == "" {
		log.Fatalf("project argument is required. See --help.")
	}
	if *instance == "" {
		log.Fatalf("instance argument is required. See --help.")
	}
	if *dbName == "" {
		log.Fatalf("dbName argument is required. See --help.")
	}

	if *dryrun {
		if !askForConfirmation("#### This is a dry-run !!! Continue?") {
			log.Fatal("Aborting")
		}
	}

	// Initialize rand
	rand.Seed(time.Now().UnixNano())

	config.init()
	if err != nil {
		log.Fatalf("error initializing: %s", err)
	}

	// Set date range for event generation
	config.TimeStart = time.Now().Round(time.Hour)
	config.TimeEnd = config.TimeStart.Add(time.Duration(*eventDateRange*24) * time.Hour)
	config.allForSaleNow = *allForSaleNow

	cssvc, err := NewCloudSpannerService(*project, *instance, *dbName)
	if err != nil {
		log.Fatalf("Couldn't connect to Google Cloud Spanner: %v", err)
	}
	defer cssvc.cleanup()

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Cleaning up resources before exiting!")
		cssvc.cleanup()
		os.Exit(1)
	}()

	switch flag.Arg(0) {
	case "create":
		if len(flag.Args()) < 2 {
			log.Fatalf("Error: create command requires schema file")
		}

		if !(*dontask || askForConfirmation("\nThis will wipe out the entire existing database.\nARE YOU SURE?")) {
			log.Fatal("Aborting")
		}
		err := cssvc.dropDB(*dbName)
		if err != nil {
			log.Println(err)
		}
		sqlb, err := ioutil.ReadFile(flag.Arg(1))
		if err != nil {
			log.Fatalf("Error reading schema file, %s", err)
		}
		err = cssvc.createDB(*dbName, string(sqlb))
		if err != nil {
			log.Fatalf("Error creating database, %s", err)
		}
		break

	case "reset":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		counters := []string{
			regionCounterTableName,
			countryCounterTableName,
			multiEventCounterTableName,
			eventCategoryCounterTableName,
			soldTicketsCounterTableName,
			accountTableName,
		}
		for _, t := range counters {
			log.Printf("clearing table %s\n", t)
			_, err := cssvc.Apply(ctx, []*spanner.Mutation{
				spanner.Delete(t, spanner.AllKeys()),
			})
			if err != nil {
				log.Fatalf("Error resetting %s: %s", t, err)
			}
		}
		gc := &generatorContext{
			cssvc: cssvc,
			chs:   make(map[string]chan map[string]*spanner.Mutation),
			count: 0,
			genStats: &generatorStats{
				EventDistr: make(map[string]*eventStats, 0),
			},
		}
		log.Printf("resetting tickets\n")
		err = genData(resetTickets, gc, *dryrun)
		if err != nil {
			log.Fatalf("genData(resetTickets) failed: %v", err)
		}

	case "venues":
		if len(flag.Args()) < 2 {
			log.Fatalf("Error: venues command requires count")
		}
		count, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			log.Fatalf("count argument must be an int")
		}
		gc := &generatorContext{
			cssvc: cssvc,
			chs:   make(map[string]chan map[string]*spanner.Mutation),
			count: count,
			genStats: &generatorStats{
				EventDistr: make(map[string]*eventStats, 0),
			},
		}
		err = genData(makeVenues, gc, *dryrun)
		if err != nil {
			log.Fatalf("genData(makeVenues) failed: %v", err)
		}

	case "tickets":
		if len(flag.Args()) < 2 {
			log.Fatalf("Error: tickets command requires count")
		}
		genStats := &generatorStats{
			EventDistr: make(map[string]*eventStats, 0),
		}

		count, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			log.Fatalf("count argument must be an int")
		}

		if count < 10e6 {
			log.Println(
				"[WARNING] It's recommended to have at least 10 million tickets" +
					" generated to conform to the desired distributions given in" +
					" the generator config!")
		}

		// generate MultiEvents and Events definitions
		venuesDB, err = loadVenuesFromDB(cssvc.Client, venuesDB)
		if err != nil {
			log.Fatalf("loadVenuesFromDB failed: %v", err)
		}

		var toGen []*multiEventGenDef
		countCommitted := int64(0)
		mets := config.weightedDists["multiEventTypes"]
		for {
			megd := &multiEventGenDef{
				multiEventType: mets.sample(),
			}

			met := config.MultiEventTypes[megd.multiEventType]
			r := met.NumEventsRange
			n := r[0]
			if len(r) > 1 {
				d := r[1] - r[0]
				n = rand.Intn(d) + r[0]
			}

			for i := 0; i < n; i++ {
				v, vt, sct, err := getRandomVenueAndSeatingConfig(megd.multiEventType)
				if err != nil {
					log.Fatal(err)
				}
				egd := &eventGenDef{
					venue: dbVenue{
						v.VenueID,
						v.VenueName,
						v.CountryCode,
					},
					venueType: vt,
				}

				for _, sc := range v.SeatingCategory {
					if sc.VenueConfig != vt || sc.SeatingConfig != sct {
						continue
					}
					egd.seatingCategories = append(egd.seatingCategories, sc)
					egd.totalSeats += sc.Seats
				}
				// check if sum of all seats + countCommitted is < count * 1.05
				if float64(megd.totalSeats+egd.totalSeats+countCommitted) >= float64(count)*1.05 {
					break
				}
				megd.events = append(megd.events, egd)
				megd.totalSeats += egd.totalSeats
			}
			if len(megd.events) >= r[0] {
				toGen = append(toGen, megd)
				countCommitted += megd.totalSeats
			}

			if float64(countCommitted) <= float64(count)*0.95 {
				continue
			}

			break
		}

		chs := make(map[string]chan map[string]*spanner.Mutation)
		for _, t := range tables {
			chs[t] = make(chan map[string]*spanner.Mutation, *batchSize*10)
		}
		bbi := &bufferedBatchInsert{
			svc:          cssvc,
			timeout:      *timeout,
			maxBatchSize: *batchSize,
			sessions:     *parallelTxn,
		}
		bbi.run(chs)

		gc := &generatorContext{
			cssvc: cssvc,
			chs:   chs,
			count: count,
			genStats: &generatorStats{
				EventDistr: make(map[string]*eventStats, 0),
			},
		}

		err = makeMultiEvents(gc, toGen)
		if err != nil {
			log.Fatalf("genData(makeMultiEvents) failed: %v", err)
		}

		for _, ch := range chs {
			close(ch)
		}
		<-bbi.done()

		log.Print("### Generator statistics ###")
		j, err := json.Marshal(genStats)
		if err != nil {
			log.Fatalf("failed to convert generator stats to json: %v", err)
		}
		log.Print(string(j))

	case "clearTables":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, t := range tables {
			log.Printf("clearing table %s\n", t)
			_, err := cssvc.Apply(ctx, []*spanner.Mutation{
				spanner.Delete(t, spanner.KeyRange{
					Start: spanner.Key{""},
					End:   spanner.Key{"x*"},
					Kind:  spanner.ClosedClosed,
				}),
			})
			if err != nil {
				log.Fatalf("Error resetting %s: %s", t, err)
			}
		}
	default:
		log.Fatalf("'%v' is not a valid command! Supported cmds are:"+
			" 'create', 'reset', 'venues', 'tickets', 'clearTables'", flag.Arg(0))
	}
}

func resetTickets(gc *generatorContext) error {
	numSeqIds := 10000
	seqNoA := rand.Perm(numSeqIds)
	skip := make(map[int]bool)
	doneCount := 0
	rowsFound := 0
	var ticketsResetCount int64
	for doneCount < numSeqIds {
		for _, i := range seqNoA {
			if skip[i] {
				continue
			}
			log.Printf("resetting tickets with seqNo: %v\n", i)
			q := fmt.Sprintf(`
				SELECT 
					EventId, CategoryId, TicketId 
				FROM Ticket 
				WHERE 
					AVAILABLE IS NULL 
					AND SeqNo = %d 
				LIMIT 30000`, i)
			rowsFound = 0
			err := gc.cssvc.read(q, func(r *spanner.Row) error {
				rowsFound++
				ticketsResetCount++
				t := &dbTicket{}
				if err := r.ToStruct(t); err != nil {
					return err
				}
				m := spanner.Update(ticketTableName,
					[]string{
						"EventId",
						"CategoryId",
						"TicketId",
						"AccountId",
						"Available",
						"PurchaseDate",
					},
					[]interface{}{t.EventID, t.CategoryID, t.TicketID, nil, true, nil})

				gc.chs[ticketTableName] <- map[string]*spanner.Mutation{t.EventID: m}
				return nil
			})
			if err != nil {
				return err
			}
			log.Printf("total reset tickets: %v, done count %v",
				ticketsResetCount, doneCount)
			if rowsFound == 0 {
				skip[i] = true
				doneCount++
			}
		}
	}
	return nil
}

func makeVenues(gc *generatorContext) error {
	log.Printf("makeVenues(%d)", gc.count)
	vcsampler := config.weightedDists["venueConfigs"]
	for i := 0; i < gc.count; i++ {
		vidB, err := uuid.New().MarshalBinary()
		if err != nil {
			return err
		}
		vid := hex.EncodeToString(vidB)
		venueType := vcsampler.sample()
		c := getCountry()
		v := &dbVenue{
			VenueID: vid,
			VenueName: c.Capital + " " +
				venueNameAdditions[rand.Intn(len(venueNameAdditions))] +
				" " + strings.Title(venueType),
			CountryCode: c.Alpha2,
		}
		err = appendMutation(gc.chs[venueTableName], venueTableName, v.VenueID, v)
		if err != nil {
			return err
		}
		err = genInfoData(gc.chs, venueInfoTableName, vid)
		if err != nil {
			return err
		}
		log.Printf("Generated Venue %v", v.VenueName)
		makeCategories(gc, v.VenueID, venueType)
	}
	return nil
}

func makeCategories(gc *generatorContext, vid string, venueType string) error {
	// create categories with seat count for each seating config
	vc := config.VenueConfig[venueType]
	diff := vc.NumSeatsRange[1] - vc.NumSeatsRange[0]
	baseSeats := rand.Intn(diff) + vc.NumSeatsRange[0]
	for sct, sc := range vc.SeatingConfigs {
		// calc seating tier distribution
		sts := config.weightedDists[venueType+"_"+sct]
		stdist := make(map[string]int)
		for i := 0; i < baseSeats*sc.SeatConfigMultiplier; i++ {
			stdist[sts.sample()]++
		}

		for st, numS := range stdist {
			// we need to split up the seating cats in batches of no more
			// than maxSeatsPerCat seats
			split := 1
			log.Printf("Generating SeatingConfig %v, Tier %v with %v seats",
				sct, st, numS)
			for numS > 0 {
				scidB, err := uuid.New().MarshalBinary()
				if err != nil {
					return err
				}
				n := maxSeatsPerCat
				if int64(numS) < n {
					n = int64(numS)
				}
				cn := fmt.Sprintf("%s %03d", st, split)
				if int64(numS) <= maxSeatsPerCat && split == 1 {
					cn = st
				}
				category := &dbSeatingCategory{
					VenueID:       vid,
					CategoryID:    hex.EncodeToString(scidB),
					VenueConfig:   venueType,
					SeatingConfig: sct,
					CategoryName:  cn,
					Seats:         n,
				}
				err = appendMutation(gc.chs[seatingCategoryTableName],
					seatingCategoryTableName, category.VenueID, category)
				if err != nil {
					return err
				}
				split++
				numS -= int(n)
			}
		}
	}
	return nil
}

func makeMultiEvents(gc *generatorContext, megds []*multiEventGenDef) error {
	log.Printf("makeMultiEvents(%d)", len(megds))
	// mets := config.weightedDists["multiEventTypes"]
	gc.genStats.NumMultiEvents += len(megds)
	for i, megd := range megds {
		log.Printf("Create ME %v of %v", i+1, len(megds))
		meidB, err := uuid.New().MarshalBinary()
		if err != nil {
			return err
		}
		me := &dbMultiEvent{
			MultiEventID: hex.EncodeToString(meidB),
			Name:         "MultiEvent " + genRandomID(),
		}
		err = appendMutation(gc.chs[multiEventTableName], multiEventTableName,
			me.MultiEventID, me)
		if err != nil {
			return err
		}
		gc.genStats.TotalDiskSpaceGB += 304 / bytesToGibi

		// met := mets.sample()
		log.Printf("Created MultiEvent Type %v", megd.multiEventType)
		err = makeEvents(gc, me.MultiEventID, megd.multiEventType,
			megd.events)
		if err != nil {
			return err
		}
	}
	return nil
}

func makeEvents(gc *generatorContext, mid string, met string, egds []*eventGenDef) error {
	log.Printf("Creating %v Event(s) ", len(egds))
	gc.genStats.NumEvents += len(egds)
	for i, egd := range egds {
		log.Printf("Generating Event %v of %v", i+1, len(egds))
		eidB, err := uuid.New().MarshalBinary()
		if err != nil {
			return err
		}
		eDate, sDate, err := config.sampleDate(5)
		if err != nil {
			return err
		}
		eid := hex.EncodeToString(eidB)
		e := &dbEvent{
			MultiEventID: mid,
			EventID:      eid,
			EventName:    "Event " + genRandomID(),
			EventDate:    eDate,
			SaleDate:     sDate,
		}
		err = appendMutation(gc.chs[eventTableName], eventTableName, e.EventID, e)
		if err != nil {
			return err
		}
		err = genInfoData(gc.chs, eventInfoTableName, eid)
		if err != nil {
			return err
		}
		gc.genStats.TotalDiskSpaceGB += (384 + float64(*infoDataSize+16)) / bytesToGibi

		log.Printf("Generating tickets for event %v in Venue %v, type: %v",
			e.EventID, egd.venue.VenueID, egd.venueType)

		ged := gc.genStats.EventDistr[met+"_"+egd.venueType]
		if ged == nil {
			ged = &eventStats{
				EventType: met,
				VenueType: egd.venueType,
			}
			gc.genStats.EventDistr[met+"_"+egd.venueType] = ged
		}
		ged.NumEvents++

		for _, sc := range egd.seatingCategories {
			log.Printf(
				"Generating %v tickets, seating config: %s, tier: %s",
				sc.Seats, sc.SeatingConfig, sc.CategoryName,
			)
			err := makeTicketCats(gc, e.EventID, *sc, config.MultiEventTypes[met])
			if err != nil {
				return err
			}

			err = makeTickets(gc, e.EventID, *sc)
			if err != nil {
				return err
			}

			ged.TotalSeats += int(sc.Seats)
		}
	}
	return nil
}

func makeTickets(gc *generatorContext, eid string, sc dbSeatingCategory) error {
	gc.genStats.NumTickets += sc.Seats
	gc.genStats.TotalDiskSpaceGB += float64(sc.Seats) * (float64(*infoDataSize+16) + 161) / bytesToGibi
	for k := int64(0); k < sc.Seats; k++ {
		tidB, err := uuid.New().MarshalBinary()
		if err != nil {
			return err
		}
		tid := hex.EncodeToString(tidB)
		t := &dbTicket{
			TicketID:   tid,
			CategoryID: sc.CategoryID,
			EventID:    eid,
			SeqNo:      k,
			Available:  true,
		}
		err = appendMutation(gc.chs[ticketTableName], ticketTableName, t.TicketID, t)
		if err != nil {
			return err
		}
		err = genInfoData(gc.chs, ticketInfoTableName, tid)
		if err != nil {
			return err
		}
	}
	return nil
}

func makeTicketCats(gc *generatorContext, eid string, sc dbSeatingCategory, met MultiEventType) error {
	pts := met.PricingRange[sc.VenueConfig]
	for pt, pr := range pts {
		if strings.HasPrefix(sc.CategoryName, pt) {
			d := pr[1] - pr[0]
			p := rand.Intn(d) + pr[0]
			gc.genStats.SumTicketPrices += float64(sc.Seats) * float64(p)

			tc := &dbTicketCategory{
				EventID:    eid,
				CategoryID: sc.CategoryID,
				Price:      float64(p) - .05,
			}
			if err := appendMutation(
				gc.chs[ticketCategoryTableName],
				ticketCategoryTableName,
				tc.CategoryID, tc); err != nil {
				return err
			}
			gc.genStats.TotalDiskSpaceGB += 96 / bytesToGibi
			break
		}
	}
	return nil
}

type generatorContext struct {
	cssvc    *CloudSpannerService
	chs      map[string]chan map[string]*spanner.Mutation
	count    int
	genStats *generatorStats
}

func genData(f func(gc *generatorContext) error, gc *generatorContext, dryrun bool) error {
	for _, t := range tables {
		gc.chs[t] = make(chan map[string]*spanner.Mutation, *batchSize*10)
	}
	bbi := &bufferedBatchInsert{
		svc:          gc.cssvc,
		timeout:      *timeout,
		maxBatchSize: *batchSize,
		sessions:     *parallelTxn,
		dryrun:       dryrun,
	}
	bbi.run(gc.chs)
	if err := f(gc); err != nil {
		return err
	}
	for _, ch := range gc.chs {
		close(ch)
	}
	<-bbi.done()
	return nil
}

func genInfoData(chs map[string]chan map[string]*spanner.Mutation, table string, id string) error {
	if !*dryrun && *infoDataSize > 0 {
		s := make([]byte, *infoDataSize)
		rand.Read(s)
		d := allocInfoStruct(table)
		if d == nil {
			return fmt.Errorf("can't alloc info struct for table %s", table)
		}
		d.set(id, s)
		if err := appendMutation(chs[table], table, id, d); err != nil {
			return err
		}
	}
	return nil
}

// askForConfirmation asks the user for confirmation. A user must type in
// "yes" or "no" and then press enter. It has fuzzy matching, so "y", "Y",
// "yes", "YES", and "Yes" all count as confirmations. If the input is not
// recognized, it will ask again. The function does not return until it gets a
// valid response from the user.
// Source: https://gist.github.com/m4ng0squ4sh/3dcbb0c8f6cfe9c66ab8008f55f8f28b
func askForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [y/n]: ", s)
		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}

func getCountry() gountries.Country {
	countrySampler := config.weightedDists["population"]
	countryName := countrySampler.sample()
	country, err := queryCountries.FindCountryByName(countryName)
	if err != nil {
		log.Printf("error finding country %v", countryName)
		return queryCountries.Countries["US"]
	}
	return country
}

func getRandomVenueAndSeatingConfig(met string) (dbVenueLoad, string, string, error) {
	nVenues := len(venuesDB)
	if nVenues == 0 {
		return dbVenueLoad{}, "", "", fmt.Errorf("no venues loaded")
	}
	var v dbVenueLoad
	var vt string
	mevscs := config.MultiEventTypes[met].VenueSeatingConfig
	l := 0
	for {
		l++
		v = venuesDB[rand.Intn(nVenues)]
		for vsc, sct := range mevscs {
			for _, sc := range v.SeatingCategory {
				vt = sc.VenueConfig
				break
			}
			if vsc == vt {
				return v, vt, sct, nil
			}
		}
		if l > 1000 {
			return dbVenueLoad{}, "", "",
				fmt.Errorf("couldn't find appropriate venue")
		}
	}
}

func genRandomID() string {
	random := rand.Intn(10000)
	return strconv.Itoa(int(time.Now().Unix())) + strconv.Itoa(random)
}
