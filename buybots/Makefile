# Copyright 2017 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PACKAGE       := spannerdemo-buybot

DATE          := $(shell TZ=UTC date +%FT%T)Z
VERSION       := $(shell git describe --always)

PROJECT       := $(shell gcloud config list --format 'value(core.project)')
GCR           ?= gcr.io

.PHONY: container push-gcr
container: 
	docker build -t $(PACKAGE):$(VERSION) -t $(PACKAGE):latest .

push-gcr: container
	# push to google cloud container registry
	docker tag $(PACKAGE):$(VERSION) $(GCR)/$(PROJECT)/$(PACKAGE):$(VERSION)
	docker tag $(PACKAGE):$(VERSION) $(GCR)/$(PROJECT)/$(PACKAGE):latest
	gcloud docker -- push $(GCR)/$(PROJECT)/$(PACKAGE):$(VERSION)
	gcloud docker -- push $(GCR)/$(PROJECT)/$(PACKAGE):latest
