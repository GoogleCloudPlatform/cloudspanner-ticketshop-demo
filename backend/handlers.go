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
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// Account is a ticket buyer account
type Account struct {
	AccountID   uuidHex  `json:"id,omitempty"`
	Name        string   `json:"name"`
	EMail       string   `json:"email"`
	CountryCode string   `json:"country"`
	Tickets     []Ticket `json:"tickets,omitempty"`
}

// MultiEvent is the event grouping object
// To group events that are of them kind like concert tours, league games,
// olympics
type MultiEvent struct {
	MultiEventID uuidHex `json:"id"`
	Name         string  `json:"name"`
	Events       []Event `json:"events,omitempty"`
}

// Venue is the event venue object
type Venue struct {
	VenueID     uuidHex `json:"id"`
	Name        string  `json:"name"`
	CountryCode string  `json:"country"`
}

// Event is the event details object
type Event struct {
	EventID     uuidHex    `json:"id"`
	VenueName   string     `json:"venue"`
	Name        string     `json:"name"`
	CountryCode string     `json:"country"`
	When        string     `json:"when"`
	Categories  []Category `json:"categories,omitempty"`
}

// Category is the category details object
type Category struct {
	CategoryID uuidHex `json:"id"`
	Name       string  `json:"name"`
	Seats      int64   `json:"seatsAvailable"`
	Price      float64 `json:"price"`
}

// Ticket is the ticket details object
type Ticket struct {
	TicketID     uuidHex `json:"id"`
	EventName    string  `json:"event,omitempty"`
	VenueName    string  `json:"venue,omitempty"`
	CategoryName string  `json:"category,omitempty"`
	PurchaseDate string  `json:"purchaseDate,omitempty"`
	Price        float64 `json:"price,omitempty"`
}

// PurchaseTicketRequest is the request to buy tickets
type PurchaseTicketRequest struct {
	AccountID  uuidHex `json:"accountId"`
	EventID    uuidHex `json:"EventId"`
	CategoryID uuidHex `json:"categoryId"`
	Quantity   int     `json:"quantity"`
}

// EventsDateRange represents the min and max event dates in the DB
type EventsDateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (id *uuidHex) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		b = b[1:]
	}
	if len(b) > 0 && b[len(b)-1] == '"' {
		b = b[:len(b)-1]
	}
	v, err := encodeHexUUIDStr(string(b))
	if err != nil {
		return err
	}
	*id = v
	return nil
}

func (id *uuidHex) MarshalJSON() ([]byte, error) {
	v, err := decodeHexUUIDStr(*id)
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

var (
	metaMutex            = &sync.Mutex{}
	meDayCacheMutex      = map[string]*sync.Mutex{}
	eventsDateRangeCache = &EventsDateRange{}
)

func apiDocs(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "apidoc/apidoc.html")
}

func (s *server) CreateAccount() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		d := json.NewDecoder(r.Body)
		defer r.Body.Close()
		a := &Account{}
		if err := d.Decode(a); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.CountryCode = strings.TrimSpace(a.CountryCode)
		a.EMail = strings.TrimSpace(a.EMail)
		a.Name = strings.TrimSpace(a.Name)
		if a.CountryCode == "" || a.EMail == "" || a.Name == "" {
			serveError(w, http.StatusUnprocessableEntity,
				"Account attributes email, country code and name are required!")
			return
		}

		_, err := countries.FindCountryByAlpha(strings.ToUpper(a.CountryCode))
		if err != nil {
			serveError(w, http.StatusNotFound, err.Error())
			return
		}
		am := accountManager{s.cssvc}
		if err := am.insert(a); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(a); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) AccountByEmail() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		am := accountManager{s.cssvc}
		a, err := am.byEmail(trimAndEscape(r, "email"))
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
		}
		if a == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err := json.NewEncoder(w).Encode(a); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) GetEventsDateRange() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if eventsDateRangeCache.From == "" {
			em := eventManager{s.cssvc}
			from, to, err := em.dateRange()
			if err != nil {
				serveError(w, http.StatusInternalServerError, err.Error())
				return
			}
			eventsDateRangeCache = &EventsDateRange{From: from, To: to}
		}

		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err := json.NewEncoder(w).Encode(eventsDateRangeCache); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) ListMultiEvents() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		d := url.QueryEscape(r.FormValue("date"))
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				log.Print(err)
				serveError(w, http.StatusBadRequest,
					"Error in date query format. Expected format: 2016-01-31")
				return
			}
		}

		var mes []*MultiEvent
		mem := multiEventManager{s.cssvc}
		if s.cacheExpiration > 0 && s.cache != nil {
			metaMutex.Lock()
			if _, ok := meDayCacheMutex[d]; !ok {
				meDayCacheMutex[d] = &sync.Mutex{}
			}
			metaMutex.Unlock()

			m := meDayCacheMutex[d]
			m.Lock()
			t, err := s.cache.Get(d)
			if err != nil {
				mes, err = mem.list(d)
				if err != nil {
					serveError(w, http.StatusInternalServerError, err.Error())
					m.Unlock()
					return
				}
				s.cache.Set(d, mes)
			} else {
				mes = t.([]*MultiEvent)
			}
			m.Unlock()
		} else {
			var err error
			mes, err = mem.list(d)
			if err != nil {
				serveError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		l := len(mes)
		if l > 200 {
			min := func(a, b int) int {
				if a <= b {
					return a
				}
				return b
			}

			i := rand.Intn(l - 200)
			mes = mes[i:min(l, i+200)]
		}

		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err := json.NewEncoder(w).Encode(mes); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) ListEvents() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := encodeHexUUIDStr(trimAndEscape(r, "meid"))
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		em := eventManager{s.cssvc}
		e, err := em.list(id)
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err = json.NewEncoder(w).Encode(e); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) GetEvent() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := encodeHexUUIDStr(trimAndEscape(r, "eid"))
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		em := eventManager{s.cssvc}
		e, err := em.get(id)
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err = json.NewEncoder(w).Encode(e); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *server) PurchaseTickets() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		d := json.NewDecoder(r.Body)
		defer r.Body.Close()
		ptr := &PurchaseTicketRequest{}
		if err := d.Decode(ptr); err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if ptr.AccountID == "" || ptr.CategoryID == "" ||
			ptr.Quantity <= 0 || ptr.Quantity > 15 {
			serveError(w, http.StatusUnprocessableEntity,
				"PurchaseTicketRequest attributes accountId and categoryId "+
					"are required and quantity must be between 1 and 15!")
			return
		}

		tm := ticketManager{s.cssvc}
		t, err := tm.buy(ptr)
		if err != nil {
			serveError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		if err = json.NewEncoder(w).Encode(t); err != nil {
			if _, ok := err.(*customTicketBackendError); !ok {
				serveError(w, http.StatusPreconditionFailed, err.Error())
				return
			}
			serveError(w, http.StatusPreconditionFailed, err.Error())
			return
		}
	}
}

func trimAndEscape(r *http.Request, v string) string {
	return url.QueryEscape(strings.TrimSpace(mux.Vars(r)[v]))
}

func serveError(w http.ResponseWriter, code int, msg string) {
	if msg == "" {
		switch code {
		default:
			msg = http.StatusText(code) + "."
		case http.StatusNotFound:
			msg = "The requested URL was not found on this server."
		case http.StatusInternalServerError:
			msg = "Sorry, something went wrong."
		}
	}
	data := &struct {
		Code        int
		Title, Text string
	}{
		Code:  code,
		Title: strings.TrimRight(msg, "."),
		Text:  msg,
	}
	debugLog.Printf(msg)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
