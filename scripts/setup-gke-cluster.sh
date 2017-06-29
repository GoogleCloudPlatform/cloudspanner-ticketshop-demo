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


USAGE="$0 [ setup NUM_CLUSTERS NODES_PER_CLUSTER | teardown NUM_CLUSTERS ]"

MACHINE_TYPE=n1-standard-2

CLUSTER_PREFIX_US=spannerdemo-us-
CLUSTER_PREFIX_EUROPE=spannerdemo-europe-
CLUSTER_PREFIX_ASIA=spannerdemo-asia-

ASIA_ZONES=(asia-east1-a asia-east1-b)
EUROPE_ZONES=(europe-west1-b europe-west1-c europe-west1-d)
US_ZONES=(us-central1-a us-central1-b us-central1-c us-central1-f)

if [[ -z "$1" ]] || [[ "$1" -ne "setup" || "$1" -ne "teardown" ]] || [[ "$1" = "setup" && $# -ne 3 ]] || [[ "$1" = "teardown" && $# -ne 2 ]]
then
    echo $USAGE
    exit 1
fi

if [ "$1" = "setup" ]
then
    echo "Creating $2 cluster with $3 nodes in each region"
    for((i=1;i<=$2;i++))
    do
        cn=$(printf "%02d" $i)
        gcloud container clusters create $CLUSTER_PREFIX_ASIA$cn \
        --num-nodes=$3 --machine-type=$MACHINE_TYPE \
        --zone ${ASIA_ZONES[($i-1)%${#ASIA_ZONES[@]}]} \
        --enable-autoscaling --min-nodes=1 --max-nodes=$(( $3 * 3 )) &

        gcloud container clusters create $CLUSTER_PREFIX_EUROPE$cn \
        --num-nodes=$3 --machine-type=$MACHINE_TYPE \
        --zone ${EUROPE_ZONES[($i-1)%${#EUROPE_ZONES[@]}]} \
        --enable-autoscaling --min-nodes=1 --max-nodes=$(( $3 * 3 )) &

        gcloud container clusters create $CLUSTER_PREFIX_US$cn \
        --num-nodes=$3 --machine-type=$MACHINE_TYPE \
        --zone ${US_ZONES[($i-1)%${#US_ZONES[@]}]} \
        --enable-autoscaling --min-nodes=1 --max-nodes=$(( $3 * 3 )) &
    done
    wait
    # setup kubeconf
    for((i=1;i<=$2;i++))
    do
        gcloud container clusters get-credentials $CLUSTER_PREFIX_ASIA$cn \
        --zone ${ASIA_ZONES[($i-1)%${#ASIA_ZONES[@]}]}
        current_context=$(kubectl config current-context)
        kubectl config set-context $CLUSTER_PREFIX_ASIA$cn --cluster $current_context --user $current_context
        kubectl config delete-context $current_context

        gcloud container clusters get-credentials $CLUSTER_PREFIX_EUROPE$cn \
        --zone ${EUROPE_ZONES[($i-1)%${#EUROPE_ZONES[@]}]}
        current_context=$(kubectl config current-context)
        kubectl config set-context $CLUSTER_PREFIX_EUROPE$cn --cluster $current_context --user $current_context
        kubectl config delete-context $current_context

        gcloud container clusters get-credentials $CLUSTER_PREFIX_US$cn \
        --zone ${US_ZONES[($i-1)%${#US_ZONES[@]}]}
        current_context=$(kubectl config current-context)
        kubectl config set-context $CLUSTER_PREFIX_US$cn --cluster $current_context --user $current_context
        kubectl config delete-context $current_context
    done


elif [ "$1" = "teardown" ]
then
    echo "Deleting $2 clusters in each region"
    for((i=1;i<=$2;i++))
    do
        cn=$(printf "%02d" $i)
        gcloud container clusters delete $CLUSTER_PREFIX_ASIA$cn \
        --zone ${ASIA_ZONES[($i-1)%${#ASIA_ZONES[@]}]} &

        kubectl config delete-cluster $CLUSTER_PREFIX_ASIA$cn &
        kubectl config delete-context $CLUSTER_PREFIX_ASIA$cn &

        gcloud container clusters delete $CLUSTER_PREFIX_EUROPE$cn \
        --zone ${EUROPE_ZONES[($i-1)%${#EUROPE_ZONES[@]}]} &

        kubectl config delete-cluster $CLUSTER_PREFIX_EUROPE$cn &
        kubectl config delete-context $CLUSTER_PREFIX_EUROPE$cn &

        gcloud container clusters delete $CLUSTER_PREFIX_US$cn \
        --zone ${US_ZONES[($i-1)%${#US_ZONES[@]}]} &

        kubectl config delete-cluster $CLUSTER_PREFIX_US$cn &
        kubectl config delete-context $CLUSTER_PREFIX_US$cn &
    done
fi