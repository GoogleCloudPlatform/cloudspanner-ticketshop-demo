#!/bin/bash
# Copyright 2018 Google Inc. All Rights Reserved.
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

apt update
apt upgrade -y
apt install -y apt-transport-https ca-certificates curl software-properties-common build-essential tmux git
# for docker install
apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 58118E89F3A912897C070ADBF76221572C52609D
apt-add-repository 'deb https://apt.dockerproject.org/repo ubuntu-zesty main'
apt update
apt install -y docker-engine
# gcloud-sdk update
export CLOUD_SDK_REPO="cloud-sdk-$(lsb_release -c -s)"
echo "deb https://packages.cloud.google.com/apt $CLOUD_SDK_REPO main" | sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
apt update && apt install -y google-cloud-sdk kubectl

PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/spanner.admin
PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/container.admin
PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/iam.serviceAccountActor
PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/iam.serviceAccountAdmin
PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/iam.serviceAccountKeyAdmin
PROJECT=`gcloud config get-value project 2> /dev/null`; gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:spanner-demo-gce-svc-acc@$PROJECT.iam.gserviceaccount.com --role roles/storage.admin

git clone https://github.com/GoogleCloudPlatform/cloudspanner-ticketshop-demo /root/cloudspanner-ticketshop-demo