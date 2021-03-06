# Cloud Spanner Ticketshop Demo Dataload Generator (TSDLG)

This repository folder contains the source code for the Ticketshop Dataload Generator (TSDLG). It is a tool to generate fake Ticketshop data and load it into Cloud Spanner.

## Building a statically linked binary containerized running on alpine

Building the statically linked binary is automated using make.

To build and package the binary into a container run `make container`.
To upload the container to Google Container Registry in the currently
set gcloud project run `make push-gcr`.

### Run from the source code dir directly

Command is the following (detailed help with `go run *.go --help`):
It's possible to use environment variables, a config `-config config.env` file or parameters.

```bash
go run *.go
```

### Run the container locally

```bash
docker run -v $PWD/<SERVICE_ACCOUNT>.json:/key.json --env-file config.env -it <BINARY_NAME>:<VERSION>
```

## Running the Ticketshop Dataload Generator on GCE

See [README.md in the root dir](../README.md#Development) on the prerequisites to
run a container on GCE.

### Setup config

The TSDLG requires a config to connect to Cloud Spanner which gets injected to the
running container via environment variables:

See `config-sample.env` as an example / template.

### Run the container

SSH into your instance and execute:

```bash
sudo su -
docker pull gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
docker run -d --env-file config.env gcr.io/<PROJECT_NAME>/<BINARY_NAME>:<VERSION>
```

To stop your container, find the running container id with `docker ps` and run `docker stop <container-id>`

## Running the Ticketshop Dataload Generator (TSDLG) in Kubernetes (preferred way)

TODO: add k8s instructions. For now see how the [setup.sh](../setup.sh) uses the
Kubernetes templates in the [k8sconfigtemplates](../k8sconfigtemplates) folder in the
repository root.

## Vendor Packaging

We use [`govendor`](https://github.com/kardianos/govendor) (`go get -u github.com/kardianos/govendor`) as the vendor package manager.

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