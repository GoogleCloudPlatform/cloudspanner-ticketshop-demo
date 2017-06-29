/*
Copyright 2017 Google Inc. All Rights Reserved.

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
	"math"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
)

const (
	soldTicketsCounterTableName   = "SoldTicketsCounter"
	regionCounterTableName        = "RegionCounter"
	countryCounterTableName       = "CountryCounter"
	multiEventCounterTableName    = "MultiEventCounter"
	eventCategoryCounterTableName = "EventCategoryCounter"
)

var (
	counters = map[string]int{
		"RegionCounter":        1e6,
		"CountryCounter":       4e4,
		"MultiEventCounter":    2e3,
		"EventCategoryCounter": 4e2,
		"SoldTicketsCounter":   5e2,
	}
	queryTimeout = time.Duration(60) * time.Second
)

// CloudSpannerService is an wrapping object of dbpath and an authenticated
// Cloud Spanner client
type CloudSpannerService struct {
	*spanner.Client
	iPath  string
	dbPath string
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

	c, err := spanner.NewClientWithConfig(ctx, dbPath, spanner.ClientConfig{NumChannels: 20})
	if err != nil {
		return nil, err
	}

	return &CloudSpannerService{c, iPath, dbPath}, nil
}

func (svc *CloudSpannerService) cleanup() {
	log.Println("Closing Google Cloud Spanner connections...")
	svc.Close()
	log.Println("Finished closing Google Cloud Spanner connections.")
}

type dbMultiEvent struct {
	MultiEventID string `spanner:"MultiEventId"`
	Name         string
}

type dbEvent struct {
	EventID     string            `spanner:"EventId"`
	VenueName   string            `spanner:"VenueName"`
	EventName   string            `spanner:"EventName"`
	CountryCode string            `spanner:"CountryCode"`
	EventDate   time.Time         `spanner:"EventDate"`
	SaleDate    time.Time         `spanner:"SaleDate"`
	Categories  []spanner.NullRow `spanner:"Categories"`
}

type dbVenue struct {
	VenueID     string `spanner:"VenueId"`
	Name        string `spanner:"Name"`
	CountryCode string `spanner:"CountryCode"`
}

type dbTicketCategory struct {
	CategoryID string  `spanner:"CategoryId"`
	Name       string  `spanner:"CategoryName"`
	Seats      int64   `spanner:"Seats"`
	Price      float64 `spanner:"Price"`
}

type dbEventTicketCategory struct {
	MultiEventID string  `spanner:"MultiEventId"`
	EventID      string  `spanner:"EventId"`
	CategoryID   string  `spanner:"CategoryId"`
	Seats        int64   `spanner:"Seats"`
	CountryCode  string  `spanner:"CountryCode"`
	Price        float64 `spanner:"Price"`
}

type dbTicket struct {
	TicketID     string `spanner:"TicketId"`
	CategoryID   string `spanner:"CategoryId"`
	EventID      string `spanner:"EventId"`
	VenueID      string `spanner:"VenueId"`
	AccountID    string `spanner:"AccountId"`
	SeqNo        int64  `spanner:"SeqNo"`
	PurchaseDate time.Time
	Available    bool
}

type dbAccount struct {
	AccountID   string               `spanner:"AccountId"`
	Name        string               `spanner:"Name"`
	EMail       string               `spanner:"EMail"`
	CountryCode string               `spanner:"CountryCode"`
	Tickets     []spanner.NullString `spanner:"Tickets"`
}

type dbCounter struct {
	Name    string  `spanner:"Name"`
	Shard   int64   `spanner:"Shard"`
	Sold    int64   `spanner:"Sold"`
	Revenue float64 `spanner:"Revenue"`
}

// strong consistent spanner query
func query(ctx context.Context, cssvc *CloudSpannerService, s spanner.Statement, f func(r *spanner.Row) error) error {
	return cssvc.Single().Query(ctx, s).Do(f)
}

// stale consistent spanner query
func staleQuery(ctx context.Context, cssvc *CloudSpannerService, s spanner.Statement, f func(r *spanner.Row) error) error {
	return cssvc.Single().WithTimestampBound(spanner.MaxStaleness(15*time.Second)).Query(ctx, s).Do(f)
}

type accountManager struct {
	cssvc *CloudSpannerService
}

// create buyer account
func (am *accountManager) insert(a *Account) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	aid, err := encodeHexUUID(uuid.New())
	if err != nil {
		return err
	}
	debugLog.Printf("creating account with id %v", aid)
	a.AccountID = aid

	acc := dbAccount{
		AccountID:   string(a.AccountID),
		Name:        url.QueryEscape(a.Name),
		CountryCode: url.QueryEscape(a.CountryCode),
		EMail:       url.QueryEscape(a.EMail),
		Tickets:     []spanner.NullString{},
	}

	im, err := spanner.InsertOrUpdateStruct("Account", acc)
	if err != nil {
		return err
	}
	st := time.Now()
	err = applyMutations(ctx, am.cssvc, []*spanner.Mutation{im})
	if err != nil {
		debugLog.Printf("Error in insertAccount %s", err.Error())
		return err
	}
	if debugLog {
		debugLog.Printf("Latency insertAcc: %v", time.Since(st))
	}
	return nil
}

// get buyer account by email
func (am *accountManager) byEmail(e string) (*Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	accs := make([]*dbAccount, 0, 1)
	stmt := spanner.Statement{
		SQL: "SELECT * FROM Account WHERE EMail=@e",
		Params: map[string]interface{}{
			"e": e,
		},
	}
	// strong read
	err := query(ctx, am.cssvc, stmt, func(r *spanner.Row) error {
		a := &dbAccount{}
		if err := r.ToStruct(a); err != nil {
			return err
		}
		accs = append(accs, a)
		return nil
	})

	if err != nil {
		debugLog.Printf("Error in fetchAccountByEmail %s", err.Error())
		return nil, err
	}

	for _, acc := range accs {
		name, err := url.QueryUnescape(acc.Name)
		if err != nil {
			return nil, err
		}
		email, err := url.QueryUnescape(acc.EMail)
		if err != nil {
			return nil, err
		}
		country, err := url.QueryUnescape(acc.CountryCode)
		if err != nil {
			return nil, err
		}

		tickets := make([]Ticket, 0, len(acc.Tickets))
		for _, v := range acc.Tickets {
			if v.Valid {
				tickets = append(tickets, Ticket{TicketID: uuidHex(v.StringVal)})
			}
		}

		return &Account{
			AccountID:   uuidHex(acc.AccountID),
			Name:        name,
			EMail:       email,
			CountryCode: country,
			Tickets:     tickets,
		}, nil
	}
	return nil, nil
}

type multiEventManager struct {
	cssvc *CloudSpannerService
}

// list all multievents by date
func (mem *multiEventManager) list(date string) ([]*MultiEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	// default to today
	var d time.Time
	if date == "" {
		d = time.Now()
	} else {
		var err error
		d, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, err
		}
	}
	stmt := spanner.Statement{
		SQL: `SELECT me.MultiEventId, me.Name 
		      FROM Event@{FORCE_INDEX=EventByDate} e, MultiEvent me 
			  WHERE e.EventDate >= @date 
				AND e.EventDate < @nextday 
				AND me.MultiEventId = e.MultiEventId 
				AND e.SaleDate <= CURRENT_TIMESTAMP() 
			  LIMIT 2500`,
		Params: map[string]interface{}{
			"date":    d,
			"nextday": d.Add(24 * time.Hour),
		},
	}

	mes := make([]*MultiEvent, 0, 2500)
	st := time.Now()
	// stale read to enable local reads in different regions
	err := staleQuery(ctx, mem.cssvc, stmt, func(r *spanner.Row) error {
		me := &dbMultiEvent{}
		if err := r.ToStruct(me); err != nil {
			return err
		}

		mes = append(mes, &MultiEvent{
			MultiEventID: uuidHex(me.MultiEventID),
			Name:         me.Name,
		})
		return nil
	})

	if err != nil {
		debugLog.Printf("Error in fetchMultiEvents %s", err.Error())
		return nil, err
	}
	// this is not 100 % accurate since it includes the processing we do per
	// row. To be more accurate would have to take the time at the first
	// execution of the row func
	debugLog.Printf("Latency fetchMEs for %v: %v", date, time.Since(st))
	return mes, nil
}

type eventManager struct {
	cssvc *CloudSpannerService
}

// get min and max event dates from DB
func (em *eventManager) dateRange() (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	stmt := spanner.Statement{
		SQL: "SELECT MIN(EventDate), MAX(EventDate) FROM Event",
	}
	from := &time.Time{}
	to := &time.Time{}
	err := staleQuery(ctx, em.cssvc, stmt, func(r *spanner.Row) error {
		if err := r.Column(0, from); err != nil {
			return err
		}
		if err := r.Column(1, to); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		debugLog.Printf("Error in getEventsDateRange %s", err.Error())
		return "", "", err
	}
	return from.Format("2006-01-02"), to.Format("2006-01-02"), nil
}

// get all events by multievent id
func (em *eventManager) list(meID uuidHex) ([]*Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	events := make([]*Event, 0, 200)
	stmt := spanner.Statement{
		SQL: `
		    SELECT DISTINCT 
		        e.EventId, e.EventName, e.EventDate, v.VenueName, v.CountryCode 
		    FROM Event@{FORCE_INDEX=EventByMultiEventId} e, 
				TicketCategory tc, 
				SeatingCategory sc, 
				Venue v 
		    WHERE e.EventId = tc.EventId 
			    AND tc.CategoryId = sc.CategoryId 
			    AND sc.VenueId = v.VenueId 
			    AND e.SaleDate <= CURRENT_TIMESTAMP() 
			    AND e.MultiEventID = @meid`,
		Params: map[string]interface{}{
			"meid": string(meID),
		},
	}
	st := time.Now()
	// stale read to enable local reads from the closest replica region
	err := staleQuery(ctx, em.cssvc, stmt, func(r *spanner.Row) error {
		e := &dbEvent{}
		if err := r.ToStruct(e); err != nil {
			return err
		}

		events = append(events, &Event{
			EventID:     uuidHex(e.EventID),
			Name:        e.EventName,
			VenueName:   e.VenueName,
			CountryCode: e.CountryCode,
			When:        e.EventDate.String(),
		})
		return nil
	})
	if err != nil {
		debugLog.Printf("Error in fetchEvents %s\n with query %s\n and meID %s",
			err.Error(), stmt, meID)
		return nil, err
	}
	// this is not 100 % accurate since it includes the processing we do per
	// row. To be more accurate would have to take the time at the first
	// execution of the row func
	if debugLog {
		debugLog.Printf("Latency fetchEs: %v", time.Since(st))
	}
	return events, nil
}

// get event by id
func (em *eventManager) get(eID uuidHex) (*Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	stmt := spanner.Statement{
		SQL: `
		    SELECT 
				e.EventId, e.EventName, e.EventDate, v.VenueName, v.CountryCode, 
				ARRAY(
					SELECT 
						STRUCT<CategoryId STRING, CategoryName STRING, 
							Price FLOAT64, Seats INT64>
						(sc.CategoryId, sc.CategoryName, tc.Price, 
							sc.Seats - (SELECT IFNULL(SUM(Sold),0) 
						FROM SoldTicketsCounter stc 
						WHERE 
							stc.EventId = tc.EventId 
							AND stc.CategoryId = tc.CategoryId)) 
					FROM 
						TicketCategory tc, 
						SeatingCategory@{FORCE_INDEX=SeatingCategoryById} sc 
					WHERE 
						tc.EventId = e.EventId 
						AND tc.CategoryId = sc.CategoryId
				) AS Categories 
			FROM 
				Event e, TicketCategory tc, 
				SeatingCategory@{FORCE_INDEX=SeatingCategoryById} sc, Venue v  
		    WHERE tc.EventId = e.EventId 
				AND tc.CategoryId = sc.CategoryId 
				AND sc.VenueId = v.VenueId 
				AND e.SaleDate <= CURRENT_TIMESTAMP() 
				AND e.EventId = @eid 
			LIMIT 1`,
		Params: map[string]interface{}{
			"eid": string(eID),
		},
	}

	var event *Event
	st := time.Now()
	err := query(ctx, em.cssvc, stmt, func(r *spanner.Row) error {
		e := &dbEvent{}
		if err := r.ToStruct(e); err != nil {
			return err
		}

		event = &Event{
			EventID:     uuidHex(e.EventID),
			Name:        e.EventName,
			VenueName:   e.VenueName,
			CountryCode: e.CountryCode,
			When:        e.EventDate.String(),
		}

		for _, dbCat := range e.Categories {
			if dbCat.Valid {
				c := &dbTicketCategory{}
				if err := dbCat.Row.ToStruct(c); err != nil {
					return err
				}
				event.Categories = append(event.Categories, Category{
					CategoryID: uuidHex(c.CategoryID),
					Name:       c.Name,
					Price:      c.Price,
					Seats:      c.Seats,
				})
			}
		}
		return nil
	})
	if err != nil {
		debugLog.Printf("Error in fetchEvent %s\nwith query %s\nand eid %s", err.Error(), stmt, eID)
		return nil, err
	}
	// this is not 100 % accurate since it includes the processing we do per
	// row. To be more accurate would have to take the time at the first
	// execution of the row func
	if debugLog {
		debugLog.Printf("Latency fetchE: %v", time.Since(st))
	}
	return event, nil
}

type ticketManager struct {
	cssvc *CloudSpannerService
}

func (tm *ticketManager) getEventTicketCat(eventID uuidHex, categoryID uuidHex) (*dbEventTicketCategory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	stmt := spanner.Statement{
		SQL: `
			SELECT 
				e.MultiEventId, e.EventId, sc.CategoryId, 
				v.CountryCode, sc.Seats, tc.Price
			FROM 
				SeatingCategory@{FORCE_INDEX=SeatingCategoryById} sc, 
				TicketCategory tc, Event e, Venue v
			WHERE tc.EventId = e.EventId AND sc.CategoryId = tc.CategoryId
				AND sc.VenueId = v.VenueId AND e.SaleDate <= CURRENT_TIMESTAMP()
				AND sc.CategoryId = @cid AND e.EventId = @eid`,
		Params: map[string]interface{}{
			"eid": string(eventID),
			"cid": string(categoryID),
		},
	}

	etc := &dbEventTicketCategory{}
	st := time.Now()
	// stale read
	err := staleQuery(ctx, tm.cssvc, stmt, func(r *spanner.Row) error { return r.ToStruct(etc) })
	if err != nil {
		debugLog.Printf("Error in getEventTicketCat %s", err.Error())
		return nil, err
	}
	// this is not 100 % accurate since it includes the processing we do per
	// row. To be more accurate would have to take the time at the first
	// execution of the row func
	debugLog.Printf("Latency getEventTicketCat: %v", time.Since(st))
	return etc, nil
}

// find 'quantity' available seats for given event and category
func (tm *ticketManager) findAvailable(eventID string, categoryID string, totalSeats int64, quantity int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	startSeq := rand.Intn(int(math.Ceil(float64(totalSeats) * 0.9)))
	retry := false
	tickets := make([]string, 0, quantity)
	// retry finding available tickets for category
	for {
		stmt := spanner.Statement{
			SQL: `
				SELECT t.TicketId 
				FROM Ticket t 
				WHERE t.EventId = @eid 
					AND t.CategoryId = @cid 
					AND t.SeqNo >= @seqno 
					AND t.Available = true 
				LIMIT @quantity`,
			Params: map[string]interface{}{
				"eid":      eventID,
				"cid":      categoryID,
				"seqno":    startSeq,
				"quantity": quantity,
			},
		}
		st := time.Now()
		err := query(ctx, tm.cssvc, stmt, func(r *spanner.Row) error {
			var tidHex string
			if err := r.Column(0, &tidHex); err != nil {
				return err
			}
			tickets = append(tickets, tidHex)
			return nil
		})
		if err != nil {
			debugLog.Printf("Error in findAvailable %s", err.Error())
			return nil, err
		}
		// this is not 100 % accurate since it includes the processing we do per
		// row. To be more accurate would have to take the time at the first
		// execution of the row func
		debugLog.Printf("Latency findAvailable: %v", time.Since(st))

		if len(tickets) < quantity {
			if retry {
				// reset tickets
				tickets = make([]string, 0, quantity)
				startSeq = 0
				retry = false
				continue
			}
			return nil, dbError(
				"OutOfTickets",
				"Couldn't find enough tickets for requested event and category.")
		}
		break
	}

	return tickets, nil
}

func fetchAndUpdateCounters(ctx context.Context, txn *spanner.ReadWriteTransaction, etc *dbEventTicketCategory, ptr *PurchaseTicketRequest, accContinent string) ([]*spanner.Mutation, error) {
	c, err := countries.FindCountryByAlpha(strings.ToUpper(etc.CountryCode))
	if err != nil {
		return nil, err
	}
	eventContinent, ok := alpha2continents[c.Continent]
	if !ok {
		return nil, dbError("CountryMapError",
			fmt.Sprintf("Couldn't map event country %s to continent",
				etc.CountryCode))
	}

	rcS := rand.Intn(counters[regionCounterTableName])
	ccS := rand.Intn(counters[countryCounterTableName])
	meS := rand.Intn(counters[multiEventCounterTableName])
	ecS := rand.Intn(counters[eventCategoryCounterTableName])
	stcS := rand.Intn(counters[soldTicketsCounterTableName])

	stmt := spanner.Statement{
		SQL: `
			SELECT 'regionCounter' AS Name, c1.Shard, c1.Sold, c1.Revenue 
			FROM RegionCounter c1 
			WHERE 
				c1.Region = @eventRegion 
				AND c1.CustomerRegion = @accRegion 
				AND c1.Shard = @c1s 
			UNION DISTINCT 
			SELECT 'countryCounter' AS Name, c2.Shard, c2.Sold, c2.Revenue 
			FROM CountryCounter c2 
			WHERE 
				c2.CountryCode = @c2c 
				AND c2.CustomerRegion = @accRegion 
				AND c2.Shard = @c2s 
			UNION DISTINCT 
			SELECT 'multiEventCounter' AS Name, c3.Shard, c3.Sold, c3.Revenue 
			FROM MultiEventCounter c3 
			WHERE 
				c3.MultiEventId = @mid 
				AND c3.CustomerRegion = @accRegion 
				AND c3.Shard = @c3s 
			UNION DISTINCT 
			SELECT 'eventCategoryCounter' AS Name, c4.Shard, c4.Sold, c4.Revenue 
			FROM EventCategoryCounter c4 
			WHERE 
				c4.EventId = @eid 
				AND c4.CategoryId = @cid 
				AND c4.CustomerRegion = @accRegion 
				AND c4.Shard = @c4s 
			UNION DISTINCT 
			SELECT 'soldTicketsCounter' AS Name, c5.Shard, c5.Sold, 0 
			FROM SoldTicketsCounter c5 
			WHERE 
				c5.EventId = @eid 
				AND c5.CategoryId = @cid 
				AND c5.Shard = @c5s`,
		Params: map[string]interface{}{
			"mid":         etc.MultiEventID,
			"eid":         etc.EventID,
			"cid":         etc.CategoryID,
			"eventRegion": eventContinent,
			"accRegion":   accContinent,
			"c1s":         rcS,
			"c2c":         strings.ToUpper(etc.CountryCode),
			"c2s":         ccS,
			"c3s":         meS,
			"c4s":         ecS,
			"c5s":         stcS,
		},
	}

	st := time.Now()
	cs := make(map[string]*dbCounter, len(counters))
	if err := txn.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		c := &dbCounter{}
		if err := r.ToStruct(c); err != nil {
			return err
		}
		cs[c.Name] = c
		return nil
	}); err != nil {
		debugLog.Printf("Error in fetchAndUpdateCounters %s", err.Error())
		return nil, err
	}
	// this is not 100 % accurate since it includes the processing we do per
	// row. To be more accurate would have to take the time at the first
	// execution of the row func
	debugLog.Printf("Latency fetchAndUpdateCounters-getCounters: %v", time.Since(st))
	m := make([]*spanner.Mutation, 0, 5)
	// update counters
	revenue := etc.Price * float64(ptr.Quantity)
	for cName := range counters {
		switch cName {
		case soldTicketsCounterTableName:
			c1, ok := cs[soldTicketsCounterTableName]
			if !ok {
				c1 = &dbCounter{
					Name:    soldTicketsCounterTableName,
					Shard:   int64(stcS),
					Sold:    0,
					Revenue: 0,
				}
			}
			m = append(m, spanner.InsertOrUpdate(c1.Name,
				[]string{"EventId", "CategoryId", "Shard", "Sold"},
				[]interface{}{
					etc.EventID,
					etc.CategoryID,
					c1.Shard,
					c1.Sold + int64(ptr.Quantity),
				}))

		case regionCounterTableName:
			c2, ok := cs[regionCounterTableName]
			if !ok {
				c2 = &dbCounter{
					Name:    regionCounterTableName,
					Shard:   int64(rcS),
					Sold:    0,
					Revenue: 0,
				}
			}
			m = append(m, spanner.InsertOrUpdate(c2.Name,
				[]string{"Region", "CustomerRegion", "Shard", "Sold", "Revenue"},
				[]interface{}{
					eventContinent,
					accContinent,
					c2.Shard,
					c2.Sold + int64(ptr.Quantity),
					c2.Revenue + revenue,
				}))

		case countryCounterTableName:
			c3, ok := cs[countryCounterTableName]
			if !ok {
				c3 = &dbCounter{
					Name:    countryCounterTableName,
					Shard:   int64(ccS),
					Sold:    0,
					Revenue: 0,
				}
			}
			m = append(m, spanner.InsertOrUpdate(c3.Name,
				[]string{
					"CountryCode",
					"CustomerRegion",
					"Shard",
					"Sold",
					"Revenue",
				},
				[]interface{}{
					strings.ToUpper(etc.CountryCode),
					accContinent,
					c3.Shard,
					c3.Sold + int64(ptr.Quantity),
					c3.Revenue + revenue,
				}))

		case multiEventCounterTableName:
			c4, ok := cs[multiEventCounterTableName]
			if !ok {
				c4 = &dbCounter{
					Name:    multiEventCounterTableName,
					Shard:   int64(meS),
					Sold:    0,
					Revenue: 0,
				}
			}
			m = append(m, spanner.InsertOrUpdate(c4.Name,
				[]string{
					"MultiEventId",
					"CustomerRegion",
					"Shard",
					"Sold",
					"Revenue",
				},
				[]interface{}{
					etc.MultiEventID,
					accContinent,
					c4.Shard,
					c4.Sold + int64(ptr.Quantity),
					c4.Revenue + revenue,
				}))

		case eventCategoryCounterTableName:
			c5, ok := cs[eventCategoryCounterTableName]
			if !ok {
				c5 = &dbCounter{
					Name:    eventCategoryCounterTableName,
					Shard:   int64(ecS),
					Sold:    0,
					Revenue: 0,
				}
			}
			m = append(m, spanner.InsertOrUpdate(c5.Name,
				[]string{
					"EventId",
					"CategoryId",
					"CustomerRegion",
					"Shard",
					"Sold",
					"Revenue",
				},
				[]interface{}{
					etc.EventID,
					etc.CategoryID,
					accContinent,
					c5.Shard,
					c5.Sold + int64(ptr.Quantity),
					c5.Revenue + revenue,
				}))

		default:
			return nil, dbError("CounterNotSupported",
				fmt.Sprintf("%s not supported", c.Name))
		}
	}
	return m, nil
}

// buy tickets for account, event and category
func (tm *ticketManager) buy(ptr *PurchaseTicketRequest) ([]*Ticket, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	// get number of total seats and price for selected category
	etc, err := tm.getEventTicketCat(ptr.EventID, ptr.CategoryID)
	if err != nil {
		return nil, err
	}
	if etc.CategoryID == "" {
		return nil, dbError("EventNotFound", "Requested event couldn't be found!")
	}

	results := make([]*Ticket, 0, ptr.Quantity)
	retryC := 0

	// retry purchase transaction with new pre-selected tickets if became
	// unavailable/sold
	for {
		tickets, err := tm.findAvailable(
			etc.EventID,
			etc.CategoryID,
			etc.Seats,
			ptr.Quantity,
		)
		if err != nil {
			return nil, err
		}

		var timingData *timingInfo
		var measStart time.Time
		_, err = tm.cssvc.ReadWriteTransaction(ctx,
			func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				measStart = time.Now()
				// check if the tickets we just selected are still available
				stmt := spanner.Statement{
					SQL: `
					SELECT t.TicketId 
					FROM Ticket t 
					WHERE t.EventId = @eid 
						AND t.CategoryId = @cid 
						AND t.TicketId IN UNNEST(@ids)`,
					Params: map[string]interface{}{
						"eid": etc.EventID,
						"cid": etc.CategoryID,
						"ids": tickets,
					},
				}
				avail := 0
				st := time.Now()
				if err := txn.Query(ctx, stmt).Do(func(r *spanner.Row) error {
					avail++
					return nil
				}); err != nil {
					return err
				}
				debugLog.Printf("Latency buyFunc-rechkAvailTickets: %v", time.Since(st))

				if avail < ptr.Quantity {
					return dbError(
						"OutOfTickets",
						"Selected tickets not available anymore.")
				}

				// fetch account info
				acc := &dbAccount{}
				stmt = spanner.Statement{
					SQL: `SELECT * FROM Account WHERE AccountId = @aid`,
					Params: map[string]interface{}{
						"aid": string(ptr.AccountID),
					},
				}
				st = time.Now()
				// strong read
				if err := txn.Query(ctx, stmt).Do(func(r *spanner.Row) error {
					if err := r.ToStruct(acc); err != nil {
						return err
					}
					return nil
				}); err != nil {
					debugLog.Printf("Error fetching account in buy func %s", err.Error())
					return err
				}
				debugLog.Printf("Latency buyFunc-getAcc: %v", time.Since(st))

				ac, err := countries.FindCountryByAlpha(strings.ToUpper(acc.CountryCode))
				if err != nil {
					return err
				}
				accContinent, ok := alpha2continents[ac.Continent]
				if !ok {
					return dbError(
						"CountryMapError",
						fmt.Sprintf("Couldn't map account country %s to continent", acc.CountryCode))
				}

				// ptr.Quantity updates on Ticket, 1 account update and 5 counter updates
				m := make([]*spanner.Mutation, 0, ptr.Quantity+len(counters)+1)
				// assign tickets
				pd := time.Now()
				for _, tidHex := range tickets {
					m = append(m,
						spanner.Update("Ticket",
							[]string{
								"EventId",
								"CategoryId",
								"TicketId",
								"AccountId",
								"PurchaseDate",
								"Available",
							},
							[]interface{}{
								etc.EventID,
								etc.CategoryID,
								tidHex,
								acc.AccountID,
								pd,
								nil,
							}))
					results = append(results,
						&Ticket{
							TicketID: uuidHex(tidHex),
						})
					acc.Tickets = append(acc.Tickets,
						spanner.NullString{
							Valid:     true,
							StringVal: tidHex,
						})
				}
				// update account with ticketIds
				m = append(m,
					spanner.Update(
						"Account",
						[]string{
							"AccountId",
							"EMail",
							"Tickets",
						},
						[]interface{}{
							acc.AccountID,
							acc.EMail,
							acc.Tickets,
						}))

				cm, err := fetchAndUpdateCounters(ctx, txn, etc, ptr, accContinent)
				if err != nil {
					return err
				}
				for _, mm := range cm {
					m = append(m, mm)
				}

				timingData = &timingInfo{
					CountryCode:  strings.ToUpper(acc.CountryCode),
					Region:       accContinent,
					RecordsCount: ptr.Quantity,
				}
				return txn.BufferWrite(m)
			})
		if err != nil {
			if retryC++; retryC < 4 {
				continue
			}
			debugLog.Printf("Error in buyFunc %s", err.Error())
			return nil, err
		}
		timingData.Latency = int(time.Since(measStart).Nanoseconds() / 1e6)
		metricsCh <- timingData
		return results, nil
	}
}

func connect(ctx context.Context, dbPath string) *spanner.Client {
	debugLog.Printf("Connecting to Spanner at %s\n", dbPath)
	st := time.Now()
	client, err := spanner.NewClient(ctx, dbPath)
	if err != nil {
		debugLog.Printf(err.Error())
	}
	debugLog.Printf("Connecting took %s \n", time.Since(st))
	return client
}

func applyMutations(ctx context.Context, cssvc *CloudSpannerService, m []*spanner.Mutation) error {
	debugLog.Printf("Insert %v rows to Spanner...", len(m))
	st := time.Now()
	_, err := cssvc.Apply(ctx, m)
	if err != nil {
		debugLog.Printf("Error: insert records from structs %s", err)
		return err
	}
	debugLog.Printf("Insert took %s \n", time.Since(st))
	return nil
}

func dbError(kind, msg string) error {
	return &customTicketBackendError{kind, msg}
}
