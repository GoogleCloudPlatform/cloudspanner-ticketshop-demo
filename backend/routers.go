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
	"net/http"
	"strings"

	"github.com/betacraft/yaag/middleware"
	"github.com/betacraft/yaag/yaag"
	"github.com/bluele/gcache"
	"github.com/gorilla/mux"
)

type server struct {
	*mux.Router
	cssvc           *CloudSpannerService
	cache           gcache.Cache
	cacheExpiration int
}

func newEndpointsRouter(cssvc *CloudSpannerService, c gcache.Cache, ce int, apidoc bool) *mux.Router {
	if apidoc {
		yaag.Init(&yaag.Config{
			On:       true,
			DocTitle: "Ticketshop Backend API",
			DocPath:  "apidoc/apidoc.html",
			BaseUrls: map[string]string{
				"Production": "",
				"Staging":    "http://localhost:8080/api",
			},
		})
	}

	r := mux.NewRouter().StrictSlash(true).PathPrefix("/api/v1").Subrouter()
	s := &server{r, cssvc, c, ce}
	for _, e := range ep {
		h := e.HandlerFunc
		if h == nil {
			h = e.ServerMethod(s)
		}
		if apidoc {
			h = middleware.HandleFunc(h)
		}
		if e.Path == "/" {
			h = e.HandlerFunc
		}
		err := r.Methods(e.Method).
			Path(e.Path).
			Name(e.Name).
			HandlerFunc(h).GetError()
		if err != nil {
			debugLog.Printf(err.Error())
		}

	}
	r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		t, err := route.GetPathTemplate()
		if err != nil {
			return err
		}

		p, err := route.GetPathRegexp()
		if err != nil {
			return err
		}
		m, err := route.GetMethods()
		if err != nil {
			return err
		}
		debugLog.Printf("%v %v %v", strings.Join(m, ","), t, p)
		return nil
	})
	return r
}

var ep = []struct {
	Name        string
	Method      string
	Path        string
	HandlerFunc http.HandlerFunc
	// ServerMethod func(*server, http.ResponseWriter, *http.Request)
	ServerMethod func(*server) func(http.ResponseWriter, *http.Request)
}{
	{
		Name:        "APIDoc",
		Method:      "GET",
		Path:        "/",
		HandlerFunc: apiDocs,
	},
	{
		Name:         "AccountCreate",
		Method:       "POST",
		Path:         "/account",
		ServerMethod: (*server).CreateAccount,
	},
	{
		Name:         "AccountDetails",
		Method:       "GET",
		Path:         "/account/{email}",
		ServerMethod: (*server).AccountByEmail,
	},
	{
		Name:         "GetEventsDateRange",
		Method:       "GET",
		Path:         "/eventsdatesrange",
		ServerMethod: (*server).GetEventsDateRange,
	},
	{
		Name:         "MultiEventList",
		Method:       "GET",
		Path:         "/multievents",
		ServerMethod: (*server).ListMultiEvents,
	},
	{
		Name:         "EventList",
		Method:       "GET",
		Path:         "/multievents/{meid}/events",
		ServerMethod: (*server).ListEvents,
	},
	{
		Name:         "EventDetails",
		Method:       "GET",
		Path:         "/events/{eid}",
		ServerMethod: (*server).GetEvent,
	},
	{
		Name:         "TicketsPurchase",
		Method:       "POST",
		Path:         "/tickets/purchase",
		ServerMethod: (*server).PurchaseTickets,
	},
}
