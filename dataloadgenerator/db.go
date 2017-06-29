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
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	adminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	"google.golang.org/grpc/codes"
)

const (
	accountTableName              = "Account"
	multiEventTableName           = "MultiEvent"
	venueTableName                = "Venue"
	venueInfoTableName            = "VenueInfo"
	eventTableName                = "Event"
	eventInfoTableName            = "EventInfo"
	ticketTableName               = "Ticket"
	ticketInfoTableName           = "TicketInfo"
	seatingCategoryTableName      = "SeatingCategory"
	ticketCategoryTableName       = "TicketCategory"
	regionCounterTableName        = "RegionCounter"
	countryCounterTableName       = "CountryCounter"
	multiEventCounterTableName    = "MultiEventCounter"
	eventCategoryCounterTableName = "EventCategoryCounter"
	soldTicketsCounterTableName   = "SoldTicketsCounter"
)

// infoSet defines an interface that allows us to set the contents of an
// Info table polymorphically.
type infoSet interface {
	set(id string, data []byte)
}

var (
	tables = []string{
		accountTableName,
		multiEventTableName,
		venueTableName,
		venueInfoTableName,
		eventTableName,
		eventInfoTableName,
		ticketTableName,
		ticketInfoTableName,
		seatingCategoryTableName,
		ticketCategoryTableName,
		regionCounterTableName,
		countryCounterTableName,
		multiEventCounterTableName,
		eventCategoryCounterTableName,
		soldTicketsCounterTableName,
	}
	infoTableTypes = map[string]infoSet{
		eventInfoTableName:  &dbEventInfo{},
		venueInfoTableName:  &dbVenueInfo{},
		ticketInfoTableName: &dbTicketInfo{},
	}
)

func allocInfoStruct(t string) infoSet {

	return infoTableTypes[t]
}

// Following are structs for database insertions/updates via the spanner client
// lib. These should correspond precisely with the table schema definitions.
type dbMultiEvent struct {
	MultiEventID string `spanner:"MultiEventId"`
	Name         string `spanner:"Name"`
}

type dbMultiEventCounter struct {
	MultiEventID   string  `spanner:"MultiEventId"`
	CustomerRegion string  `spanner:"CustomerRegion"`
	Shard          int64   `spanner:"Shard"`
	Sold           int64   `spanner:"Sold"`
	Revenue        float64 `spanner:"Revenue"`
}

type dbVenue struct {
	VenueID     string `spanner:"VenueId"`
	VenueName   string `spanner:"VenueName"`
	CountryCode string `spanner:"CountryCode"`
}

type dbVenueInfo struct {
	VenueID string `spanner:"VenueId"`
	Data    []byte `spanner:"Data"`
}

func (d *dbVenueInfo) set(id string, data []byte) {
	d.VenueID = id
	d.Data = data
}

type dbVenueLoad struct {
	VenueID         string               `spanner:"VenueId"`
	VenueName       string               `spanner:"VenueName"`
	CountryCode     string               `spanner:"CountryCode"`
	SeatingCategory []*dbSeatingCategory `spanner:"SeatingCategory"`
}

type dbSeatingCategory struct {
	VenueID       string `spanner:"VenueId"`
	CategoryID    string `spanner:"CategoryId"`
	VenueConfig   string `spanner:"VenueConfig"`
	SeatingConfig string `spanner:"SeatingConfig"`
	CategoryName  string `spanner:"CategoryName"`
	Seats         int64  `spanner:"Seats"`
}

type dbAccount struct {
	AccountID   string               `spanner:"AccountId"`
	Name        string               `spanner:"Name"`
	EMail       string               `spanner:"Email"`
	CountryCode string               `spanner:"CountryCode"`
	Tickets     []spanner.NullString `spanner:"Tickets,omitempty"`
}

type dbEvent struct {
	EventID      string    `spanner:"EventId"`
	MultiEventID string    `spanner:"MultiEventId"`
	EventName    string    `spanner:"EventName"`
	EventDate    time.Time `spanner:"EventDate"`
	SaleDate     time.Time `spanner:"SaleDate"`
}

type dbEventInfo struct {
	EventID string `spanner:"EventId"`
	Data    []byte `spanner:"Data"`
}

func (d *dbEventInfo) set(id string, data []byte) {
	d.EventID = id
	d.Data = data
}

type dbTicketCategory struct {
	EventID    string  `spanner:"EventId"`
	CategoryID string  `spanner:"CategoryId"`
	Price      float64 `spanner:"Price"`
}

type dbEventCategoryCounter struct {
	EventID        string  `spanner:"EventId"`
	CategoryID     string  `spanner:"CategoryId"`
	CustomerRegion string  `spanner:"CustomerRegion"`
	Shard          int64   `spanner:"Shard"`
	Sold           int64   `spanner:"Sold"`
	Revenue        float64 `spanner:"Revenue"`
}

type dbTicket struct {
	TicketID     string           `spanner:"TicketId"`
	CategoryID   string           `spanner:"CategoryId"`
	EventID      string           `spanner:"EventId"`
	AccountID    string           `spanner:"AccountId"`
	SeqNo        int64            `spanner:"SeqNo"`
	PurchaseDate spanner.NullTime `spanner:"PurchaseDate"`
	Available    bool             `spanner:"Available"`
}

type dbTicketInfo struct {
	TicketID string `spanner:"TicketId"`
	Data     []byte `spanner:"Data"`
}

func (d *dbTicketInfo) set(id string, data []byte) {
	d.TicketID = id
	d.Data = data
}

type dbRegionCounter struct {
	Region         string  `spanner:"Region"`
	CustomerRegion string  `spanner:"CustomerRegion"`
	Shard          int64   `spanner:"Shard"`
	Sold           int64   `spanner:"Sold"`
	Revenue        float64 `spanner:"Revenue"`
}

type dbCountryCounter struct {
	CountryCode    string  `spanner:"CountryCode"`
	CustomerRegion string  `spanner:"CustomerRegion"`
	Shard          int64   `spanner:"Shard"`
	Sold           int64   `spanner:"Sold"`
	Revenue        float64 `spanner:"Revenue"`
}

func loadVenuesFromDB(client *spanner.Client, vs []dbVenueLoad) ([]dbVenueLoad, error) {
	log.Printf("loading venues...")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	stmt := spanner.Statement{
		SQL: `
		SELECT 
			VenueId, 
			ARRAY(
				SELECT 
					STRUCT<
						CategoryId STRING, 
						CategoryName STRING, 
						VenueConfig STRING, 
						SeatingConfig STRING, 
						Seats INT64>(sc.CategoryId, sc.CategoryName, 
						sc.VenueConfig, sc.SeatingConfig, sc.Seats) 
				FROM SeatingCategory sc 
				WHERE sc.VenueId = v.VenueId
			) as SeatingCategory 
		FROM Venue v`,
	}
	if err := client.Single().Query(ctx, stmt).Do(func(r *spanner.Row) error {
		v := &dbVenueLoad{}
		if err := r.ToStruct(v); err != nil {
			return err
		}
		vs = append(vs, *v)
		return nil
	}); err != nil {
		return nil, err
	}
	log.Printf("%v venues loaded from DB", len(vs))
	return vs, nil
}

type eventCategory struct {
	EventID    string `spanner:"EventId"`
	CategoryID string `spanner:"CategoryId"`
}

// CloudSpannerService is an wrapping object of dbpath and an authenticated
// Cloud Spanner client
type CloudSpannerService struct {
	*spanner.Client
	aClient *database.DatabaseAdminClient
	iPath   string
	dbPath  string
}

// NewCloudSpannerService creates new authenticated Cloud Spanner client with
// the given service account. If no service account is provided, the default
// auth context is used.
func NewCloudSpannerService(project, instance, db string) (*CloudSpannerService, error) {
	iPath := fmt.Sprintf("projects/%v/instances/%v",
		strings.TrimSpace(project),
		strings.TrimSpace(instance),
	)
	dbPath := fmt.Sprintf("%v/databases/%v",
		iPath,
		strings.TrimSpace(db),
	)
	log.Printf("Connecting Spanner client to %s\n", dbPath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := spanner.NewClientWithConfig(
		ctx,
		dbPath,
		spanner.ClientConfig{NumChannels: 20},
	)
	if err != nil {
		return nil, err
	}
	ac, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		return nil, err
	}

	return &CloudSpannerService{c, ac, iPath, dbPath}, nil

}

func (svc *CloudSpannerService) cleanup() {
	log.Println("Closing Google Cloud Spanner connections...")
	svc.Close()
	svc.aClient.Close()
	log.Println("Finished closing Google Cloud Spanner connections.")
}

func (svc *CloudSpannerService) createDB(name, schema string) error {
	var stmts []string
	if len(schema) > 0 {
		stmtsRaw := strings.Split(schema, ";")
		for _, s := range stmtsRaw {
			s = strings.TrimSpace(s)
			if len(s) > 0 && !strings.HasPrefix(s, "--") {
				stmts = append(stmts, s)
			}
		}
		fmt.Printf("stmts: %v\n", stmts)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	op, err := svc.aClient.CreateDatabase(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          svc.iPath,
		CreateStatement: "CREATE DATABASE `" + name + "`",
		ExtraStatements: stmts,
	})
	if err != nil {
		return err
	}
	_, err = op.Wait(ctx)
	return err
}

func (svc *CloudSpannerService) dropDB(name string) error {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	err := svc.aClient.DropDatabase(ctx, &adminpb.DropDatabaseRequest{
		Database: name,
	})
	return err
}

func (svc *CloudSpannerService) read(sql string, f func(r *spanner.Row) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()
	stmt := spanner.Statement{
		SQL: sql,
	}
	return svc.Single().Query(ctx, stmt).Do(f)
}

func (svc *CloudSpannerService) clearTable(table string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_, err := svc.Apply(ctx, []*spanner.Mutation{
		spanner.Delete(table, spanner.AllKeys()),
	})
	return err
}

func appendMutation(ch chan<- map[string]*spanner.Mutation, table, key string, s interface{}) error {
	im, err := spanner.InsertOrUpdateStruct(table, s)
	if err != nil {
		return err
	}
	ch <- map[string]*spanner.Mutation{key: im}
	return nil
}

type job struct {
	ms         []*spanner.Mutation
	retryCount int
	created    time.Time
}

type bufferedBatchInsert struct {
	svc          *CloudSpannerService
	sessions     int
	maxBatchSize int
	timeout      int
	dryrun       bool
	donec        chan struct{}

	jobs     chan *job
	highPrio chan *job
	lowPrio  chan *job
	jobsWG   sync.WaitGroup
}

func (b *bufferedBatchInsert) worker(jobs, highPrio, lowPrio <-chan *job) {
	for {
		var j, hpj, lpj *job
		select {
		case hpj = <-highPrio:
			b.applyMutations(hpj)
		case j = <-jobs:
			for len(highPrio) > 0 {
				b.applyMutations(<-highPrio)
			}
			b.applyMutations(j)
		case lpj = <-lowPrio:
			for len(highPrio) > 0 || len(jobs) > 0 {
				if len(highPrio) > 0 {
					b.applyMutations(<-highPrio)
				}
				if len(jobs) > 0 {
					b.applyMutations(<-jobs)
				}
			}
			b.applyMutations(lpj)
		}
	}
}

func (b *bufferedBatchInsert) run(chs map[string]chan map[string]*spanner.Mutation) {
	b.donec = make(chan struct{})
	b.jobs = make(chan *job, b.sessions*2)
	b.highPrio = make(chan *job, b.sessions*10)
	b.lowPrio = make(chan *job, b.sessions*10)
	for s := 0; s < b.sessions; s++ {
		go b.worker(b.jobs, b.highPrio, b.lowPrio)
	}
	go func() {
		// channels WaitGroup
		var wg sync.WaitGroup
		for n, ch := range chs {
			wg.Add(1)
			go func(name string, c <-chan map[string]*spanner.Mutation, wg *sync.WaitGroup) {
				buf := &buff{
					b:       make(map[string][]*spanner.Mutation),
					touched: time.Now(),
				}
				go func(buf *buff) {
					for {
						time.Sleep(100 * time.Millisecond)
						if buf.bufSize == 0 ||
							time.Since(buf.touched) < 1*time.Second {
							continue
						}
						b.flush(buf)
						log.Printf("Timed flush for channel %v", name)
					}
				}(buf)
				for mm := range c {
					for k, m := range mm {
						buf.add(k, m)
						if buf.bufSize >= b.maxBatchSize*100 {
							b.flush(buf)
						}
					}
				}
				if buf.bufSize > 0 {
					b.flush(buf)
				}
				wg.Done()
				log.Printf(
					"Remaining jobs in channel %v jobs: %v and highPrio: %v "+
						"and lowPrio: %v ",
					name, len(b.jobs), len(b.highPrio), len(b.lowPrio))
			}(n, ch, &wg)
		}
		wg.Wait()
		b.jobsWG.Wait()
		close(b.donec)
		log.Print("Finished all jobs")
	}()
}

func (b *bufferedBatchInsert) done() <-chan struct{} {
	return b.donec
}

func (b *bufferedBatchInsert) flush(buf *buff) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	buf.touched = time.Now()

	if buf.bufSize == 0 {
		return
	}

	for _, ms := range buf.sortedBatches(b.maxBatchSize) {
		b.jobs <- &job{ms: ms, created: time.Now()}
		b.jobsWG.Add(1)
	}

	buf.reset()
}

type buff struct {
	b       map[string][]*spanner.Mutation
	bufSize int
	mu      sync.Mutex
	touched time.Time
}

func (b *buff) sortedBatches(batchSize int) [][]*spanner.Mutation {
	if b.bufSize == 0 {
		return nil
	}

	var keys []string
	for k := range b.b {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sbs [][]*spanner.Mutation
	var ms []*spanner.Mutation
	for _, k := range keys {
		for _, m := range b.b[k] {
			ms = append(ms, m)

			if len(ms) >= batchSize {
				sbs = append(sbs, ms)
				ms = nil
			}
		}
	}
	sbs = append(sbs, ms)

	return sbs
}

func (b *buff) add(k string, v *spanner.Mutation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.touched = time.Now()
	b.bufSize++
	if _, ok := b.b[k]; ok {
		b.b[k] = append(b.b[k], v)
		return
	}
	b.b[k] = []*spanner.Mutation{v}
	return
}

func (b *buff) reset() {
	b.b = make(map[string][]*spanner.Mutation)
	b.bufSize = 0
}

func (b *bufferedBatchInsert) applyMutations(j *job) {
	if b.dryrun {
		b.jobsWG.Done()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(b.timeout)*time.Second)
	defer cancel()

	_, err := b.svc.Apply(ctx, j.ms, spanner.ApplyAtLeastOnce())

	if err == nil {
		b.jobsWG.Done()
		return
	}

	log.Printf("Error inserting records, retrying: %s", err.Error())
	if spanner.ErrCode(err) == codes.DeadlineExceeded ||
		spanner.ErrCode(err) == codes.Canceled {
		select {
		case b.highPrio <- j:
		default:
			log.Printf("HighPrio jobs channel is full, discarding retry: %v", err)
			b.jobsWG.Done()
		}
		// Slow down push on Cloud Spanner a bit
		time.Sleep(time.Duration(rand.Intn(250)) * time.Millisecond)
		return
	}

	if spanner.ErrCode(err) == codes.NotFound {
		if time.Since(j.created) > 30*time.Minute {
			log.Printf("Couldn't persist job with 'NotFound' error for 30 min, discarding")
			b.jobsWG.Done()
			return
		}
		select {
		case b.lowPrio <- j:
		default:
			log.Printf("LowPrio jobs channel is full, discarding retry: %v", err)
			b.jobsWG.Done()
		}
		return
	}

	j.retryCount++

	if j.retryCount > 10 {
		log.Printf("Tried 10 times to persist without success. Last err: %v", err)
		b.jobsWG.Done()
		return
	}

	select {
	case b.highPrio <- j:
	default:
		log.Printf("HighPrio jobs channel is full, discarding retry: %v", err)
		b.jobsWG.Done()
	}
}
