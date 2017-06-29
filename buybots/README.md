# Cloud Spanner Ticketshop Demo BuyBot (TSBB)

This repository folder contains the source code for the Ticketshop BuyBot (TSBB). It simulates users buying tickets by calling the endpoint APIs from the Ticketshop Backend (TSBE) application.

## Build a containerized Ticketshop BuyBot

To build and package the python TSBB into a container run `make container`.
To upload the container to Google Container Registry in the currently
set gcloud project run `make push-gcr`.

## Run the Ticketshop BuyBot container locally

To run the TSBB container locally you need to copy and adjust the `config-sample.env` and save it under `config.env`.

Example `config.env` when you run the demo locally:
```bash
ROOT_URL=http://localhost:8090/api/v1
CONTINUOUS=1
COUNTRIES=DE
```

```bash
docker run --env-file config.env -it <BINARY_NAME>:<VERSION>
```

## Running the Ticketshop BuyBot on GCE

See [README.md in the root dir](../README.md#Development) on the prerequisites to
run a container on GCE.

### Setup config

The TSBB requires a config to connect to the TicketShop Backend (TSBE) which gets
injected to the running container via environment variables:

See `config-sample.env` as an example / template.

### Run the container

SSH into your instance and execute:

```bash
sudo su -
docker pull gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
docker run -d --env-file config.env gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
```

To stop your container, find the running container id with `docker ps` and run `docker stop <container-id>`

## Running the Ticketshop BuyBot (TSBB) in Kubernetes (preferred way)

TODO: add k8s instructions. For now see how the [setup.sh](../setup.sh) uses the
Kubernetes templates in the [k8sconfigtemplates](../k8sconfigtemplates) folder in the
repository root.

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