# Cloud Spanner Ticketshop Demo Backend (TSBE)

This repository folder contains the source code for the Ticketshop Backend application. It provides a RESTful API for the [Ticketshop BuyBot](../buybot/README.md) (TSBB) using Cloud Spanner as the storage backend. The API documentation can be found int the folder apidoc. The backend is mostly stateless and only keeps a configurable cache for event lookups. Metrics that are collected during transactions are asynchronously published to InfluxDB to be consumed by the [Ticketshop Dashboard](../dashboard/README.md) (TSDB).

## Development

Uses your gcloud auth context, make sure your current user has access to the project
You might need to run `gcloud auth login` after you switched to the user you want to use.
The program uses default credentials from gcloud sdk env if no GOOGLE_APPLICATION_CREDENTIALS
environment variable, pointing to a service account key.json, is set.

## Building a statically linked binary containerized running on alpine

Building the statically linked binary is automated using make.

To build and package the binary into a container run `make container`.
To upload the container to Google Container Registry in the currently
set gcloud project run `make push-gcr`.

## Running the Ticketshop Backend locally

### Create a Service Account

Refer to the Google Cloud Documentation how to create a service account for Spanner.
The service account from [`dataloadgenerator`](../dataloadgenerator/README.md) can be reused here.

### Run from the source code dir directly

Command is the following (detailed help with `go run *.go --help`):
It's possible to use environment variables, a config `-config config.env` file or parameters.

```bash
go run *.go
```

### Run the container locally

```bash
docker run -v $PWD/<SERVICE_ACCOUNT>.json:/key.json -p 8090:8090 --env-file config.env -it <BINARY_NAME>:<VERSION>
```

## Running the Ticketshop Backend on GCE

See [README.md in the root dir](../README.md#Development) on the prerequisites to
run a container on GCE.

### Setup config

The ticketshop backend requires a config to connect to Cloud Spanner and InfluxDB which gets
injected to the running container via environment variables:

See `config-sample.env` as an example / template.

The APIDOC parameter enables the generation of APIDocs during development. See section API Docs.
Defaults are `APIDOC=false` and `DEBUG=false`.

### Run the container

SSH into your instance and execute:

```bash
sudo su -
docker pull gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
docker run -d -p 8090:8090 --env-file config.env gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
```

To stop your container, find the running container id with `docker ps` and run `docker stop <container-id>`

## Running the Ticketshop Backend (TSBE) in Kubernetes (preferred way)

TODO: add k8s instructions. For now see how the [setup.sh](../setup.sh) uses the
Kubernetes templates in the [k8sconfigtemplates](../k8sconfigtemplates) folder in the
repository root.

### One time setup

1. Create configmap and secret

```bash
kubectl apply -f ticketbackend-secret.yaml
kubectl apply -f ticketbackend-config.yaml
```

To switch project, instance and/or database edit the config and rerun `kubectl apply -f  ticketdatagen-config.yaml`.
To apply the new config, just scale down the TSBE deployment to 0 and back up to N.

### Running and scaling the TSBE

```bash
kubectl apply -f ticketbackend-deployment.yaml
kubectl apply -f ticketbackend-service.yaml
```

To scale the backend do `kubectl scale deployment ticketbackend --scale=N`

## API Docs

We use [`yaag`](https://github.com/betacraft/yaag) to generate API Docs.

To update the API docs add `-v $PWD/apidoc:/apidoc` when running the container.

## Vendor Packaging

We use [`govendor`](https://github.com/kardianos/govendor) (`go get -u github.com/kardianos/govendor`) as the vendor package manager.

## Queries used during ticket buying flow

What are the steps involving buying 1 or more tickets for an event:

### List all MultiEvents (ME) on specific date

```sql
SELECT me.MultiEventId, me.Name FROM Event@{FORCE_INDEX=EventByDate} e, MultiEvent me WHERE e.EventDate >= @date AND e.EventDate < @nextday AND me.MultiEventId = e.MultiEventId AND e.SaleDate <= CURRENT_TIMESTAMP() LIMIT 2500
```

 *WithTimestampBound(spanner.MaxStaleness(15*time.Second))*

### List all events for ME including venue, seating and ticket categories

```sql
SELECT DISTINCT e.EventId, e.EventName, e.EventDate, v.VenueName, v.CountryCode FROM Event@{FORCE_INDEX=EventByMultiEventId} e, TicketCategory tc, SeatingCategory sc, Venue v WHERE e.EventId = tc.EventId AND tc.CategoryId = sc.CategoryId AND sc.VenueId = v.VenueId AND e.SaleDate <= CURRENT_TIMESTAMP() AND e.MultiEventID = @mid
```

 *WithTimestampBound(spanner.MaxStaleness(15*time.Second))*

### List number of available tickets per SeatingCategory for an event

```sql
SELECT
  e.EventId, e.EventName, e.EventDate, v.VenueName, v.CountryCode,
  ARRAY(SELECT STRUCT<CategoryId STRING, CategoryName STRING, Price FLOAT64, Seats INT64>(sc.CategoryId, sc.CategoryName, tc.Price, sc.Seats - (SELECT IFNULL(SUM(Sold),0) FROM SoldTicketsCounter stc WHERE stc.EventId = tc.EventId AND stc.CategoryId = tc.CategoryId)) FROM TicketCategory tc, SeatingCategory@{FORCE_INDEX=SeatingCategoryById} sc WHERE tc.EventId = e.EventId AND tc.CategoryId = sc.CategoryId) AS Categories
FROM
  Event e,
  TicketCategory tc,
  SeatingCategory@{FORCE_INDEX=SeatingCategoryById} sc,
  Venue v
WHERE
  tc.EventId = e.EventId AND tc.CategoryId = sc.CategoryId AND sc.VenueId = v.VenueId AND e.SaleDate <= CURRENT_TIMESTAMP() AND e.EventId = @eid LIMIT 1
```

### Ticket purchase

```sql
SELECT N tickets (1 <= N <= 10) FROM randomly chosen category and update tickets: set AccountId + `available` to `false` for those tickets, add TicketIds to TicketsArray under Account + Update counters - Mixed Stale Read + Strong Read+WriteTransaction
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md)

## License

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

*This is not an official Google product*