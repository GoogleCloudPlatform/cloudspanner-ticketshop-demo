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

USAGE="$0 NUM_CLUSTERS N"
CLUSTER_PREFIX=spannerdemo-
BINARY=spannerdemo-buybot

if [[ $# -ne 2 ]]
then
  echo $USAGE
  exit 1
fi

for((i=1;i<=$1;i++)); do
  for region in "asia" "europe" "us"; do
    cn=$(printf "%02d" $i)
    echo "=== Configuring $CLUSTER_PREFIX$region-$cn ==="
    kubectl --context $CLUSTER_PREFIX$region-$cn scale deployment $BINARY-$region --replicas "$2"
  done
done