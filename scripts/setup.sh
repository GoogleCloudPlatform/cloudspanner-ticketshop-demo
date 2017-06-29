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

set -e
DATE=$(TZ=UTC date '+%Y-%m-%d_%H%M')
VERSION=$(git describe --always)

K8S_CONFIGS_TMPLS_FOLDER='k8sconfigtemplates/'
K8S_GENERATED_CONFIGS_FOLDER='k8sgenerated/'
K8S_DATALOADCONFIG_YAMLTMPL='spannerdemo-dataload-config'
K8S_SECRET_YAMLTMPL='spannerdemo-secret'
K8S_VENUESLOAD_YAMLTMPL='spannerdemo-venuesload-job'
K8S_TICKETSLOAD_YAMLTMPL='spannerdemo-ticketsload-job'
K8S_RESET_YAMLTMPL='spannerdemo-reset-job'
K8S_BACKEND_INFLUX_YAMLTMPL='spannerdemo-backend-influx'
K8S_BACKEND_YAMLTMPL='spannerdemo-backend'
K8S_BUYBOTS_YAMLTMPL='spannerdemo-buybots'
K8S_DASHBOARD_YAMLTMPL='spannerdemo-dashboard'

CLUSTER_PREFIX=spannerdemo-

P=`gcloud config get-value project 2> /dev/null`
R=`gcloud config get-value compute/region 2> /dev/null`
Z=`gcloud config get-value compute/zone 2> /dev/null`

# check if project is correct
read -p "Select project [$P]: " project
project=${project:-$P}
gcloud config set project $project
echo "Project set to '$project'"
printf "\n"


# gcloud compute zones list

# # check and ask for region/zone
# read -p "Select compute region [$R]: " region
# region=${region:-$R}
# echo "Compute Region is set to '$region'"
# printf "\n"

# read -p "Select compute zone [$Z]: " zone
# zone=${zone:-$Z}
# echo "Compute Zone is set to '$zone'"
# printf "\n"

# read -a instanceconfigs <<<$(gcloud spanner instance-configs list --format 'value[separator=":"](displayName)')
instanceconfigs=("regional-asia-east1" "regional-europe-west1" "regional-us-central1")

# ask for spanner instance config
printf "Please select a config for the Spanner instance.\n"
PS3='Your choice: '
select sconfig in "${instanceconfigs[@]}"
do
     if [[ -n $sconfig ]]; then
        break
     else
        printf "Invalid choice! \n"
     fi
done
echo "Spanner Instance config set to '$sconfig'"
printf "\n"

case $sconfig in
    "regional-asia-east1")
        primary_region="asia"
        primary_zone="asia-east1-c"
        instanceconf="regional-asia-east1"
        ;;
    # "asia-northeast1")
    #     primary_region="asia"
    #     primary_zone="asia-northeast1-b"
    #     instanceconf="regional-asia-northeast1"
    #     ;;
    "regional-europe-west1")
        primary_region="europe"
        primary_zone="europe-west1-d"
        instanceconf="regional-europe-west1"
        ;;
    "regional-us-central1")
        primary_region="us"
        primary_zone="us-central1-b"
        instanceconf="regional-us-central1"
        ;;
    *) echo invalid option exit 1
        ;;
esac

# echo "Compute Region is set to '$region'"
# echo "Compute Zone is set to '$primary_zone'"

# ask for spanner instance name
while [[ -z "$instance" || ${#instance} -lt 6 ]]; do
    read -p "Spanner Instance Name (min 6 chars): " instance
    echo "Spanner Instance name set to '$instance'"
    printf "\n"
done

# ask for db name
while [[ -z "$db" ]]; do
    read -p "DB Name: " db
    echo "Database name set to '$db'"
    printf "\n"
done 

# ask for size of cluster 100GB / 1TB / 10 TB / 20 TB
printf "Please select a size for the demo DB.\n"
PS3='Your choice: '
options=("100GB" "1TB" "10TB")
select size in "${options[@]}"
do
    case $size in
        "100GB")
            spannernodes=1
            gkenodes=3
            venues=1000
            tickets=20000000
            filldatasize=6000
            batchsize=200
            eventdaterange=100
            break
            ;;
        "1TB")
            spannernodes=3
            gkenodes=5
            venues=1000
            tickets=100000000
            filldatasize=12000
            batchsize=100
            eventdaterange=100
            break
            ;;
        "10TB")
            spannernodes=10
            gkenodes=10
            venues=10000
            tickets=1000000000
            filldatasize=12000
            batchsize=100
            eventdaterange=200
            break
            ;;
         *) echo invalid option;;
    esac
done
echo "Demo Database Size set to '$size'"
printf "\n"


printf "This script will create a new $spannernodes-node(s) Spanner Instance named '$instance' in '$sconfig' of '$size' size,\n"\
"a GKE cluster in each region\n"\
"a service account named '$instance-$db-$sconfig-key@...' with access to Spanner. \n\n"\
"Make sure none of the resources is already existing!\n\n"

read -r -p "Are you sure you want to proceed? [y/N] " response
case "$response" in
    [yY][eE][sS]|[yY]) 
        # now do the meaty stuff
        set -o xtrace
        printf "Create Spanner Instance...\n"
        gcloud beta spanner instances create $instance --config $instanceconf --description "Spanner Demo Instance" --nodes $spannernodes

        printf "Create GKE cluster...\n"
        scripts/setup-gke-cluster.sh setup 1 $gkenodes
        # gcloud container clusters create $instance-$db-load-cluster --num-nodes=$gkenodes --machine-type=$gkemachinetype --zone $zone
        
        printf "Create Service Account...\n"
        gcloud iam service-accounts create $instance-$db --display-name "Cloud Spanner Demo Service Account - generated"
        gcloud iam service-accounts keys create $instance-$db-$sconfig-key.json --iam-account $instance-$db@$project.iam.gserviceaccount.com
        gcloud projects add-iam-policy-binding $project --member serviceAccount:$instance-$db@$project.iam.gserviceaccount.com --role roles/spanner.admin
        
        printf "Build Application Containers for datagenerator, backend, buybot, and dashboard and push to GCR"
        
        cd dataloadgenerator && make push-gcr && cd ..
        cd buybots && make push-gcr && cd ..
        cd backend && make push-gcr && cd ..
        cd dashboard && make push-gcr && cd ..

        printf "Generating Kubernetes Configuration files"

        printf "Generate K8S configs..."
        mkdir -p $K8S_GENERATED_CONFIGS_FOLDER
        
        set +o xtrace
        # K8S SPANNERDEMO LOAD CONFIG
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{SPANNER_INSTANCE}}/'$instance'/' -e 's/{{SPANNER_DATABASE}}/'$db'/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_DATALOADCONFIG_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_DATALOADCONFIG_YAMLTMPL-$instance-$db-$VERSION.yaml 

        # K8S SPANNERDEMO SECRET
        KEYFILE_BASE64=$(base64 -w 0 $instance-$db-$sconfig-key.json)
        sed -e 's/{{BASE64_KEY_JSON}}/'$KEYFILE_BASE64'/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_SECRET_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_SECRET_YAMLTMPL-$instance-$db-$VERSION.yaml

        # K8S SPANNERDEMO LOAD VENUES
        NUM_VENUES_PER_POD=1000
        COMPLETIONS=$[venues/$NUM_VENUES_PER_POD]
        if [ $COMPLETIONS -lt 1 ]; then
            COMPLETIONS=1
        fi
        PARALLELISM=$[spannernodes*3]
        if [ $PARALLELISM -gt $COMPLETIONS ]; then
            PARALLELISM=$COMPLETIONS
        fi
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{VENUES_JOB_COMPLETIONS}}/'$COMPLETIONS'/' -e 's/{{VENUES_JOB_PARALLELISM}}/'$PARALLELISM'/' -e 's/{{NUM_VENUES_PER_POD}}/'$NUM_VENUES_PER_POD'/' \
        -e 's/{{BATCHSIZE}}/'$batchsize'/' -e 's/{{INFODATASIZE}}/'$filldatasize'/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_VENUESLOAD_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_VENUESLOAD_YAMLTMPL-$instance-$db-$VERSION.yaml

        # K8S SPANNERDEMO LOAD TICKETS
        NUM_TICKETS_PER_POD=10000000
        COMPLETIONS=$[tickets/$NUM_TICKETS_PER_POD]
        PARALLELISM=$spannernodes
        if [ $PARALLELISM -gt $COMPLETIONS ]; then
            PARALLELISM=$COMPLETIONS
        fi
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{TICKETS_JOB_COMPLETIONS}}/'$COMPLETIONS'/' -e 's/{{TICKETS_JOB_PARALLELISM}}/'$PARALLELISM'/' -e 's/{{NUM_TICKETS_PER_POD}}/'$NUM_TICKETS_PER_POD'/' \
        -e 's/{{BATCHSIZE}}/'$batchsize'/' -e 's/{{INFODATASIZE}}/'$filldatasize'/' \
        -e 's/{{EVENTDATERANGE}}/'$eventdaterange'/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_TICKETSLOAD_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_TICKETSLOAD_YAMLTMPL-$instance-$db-$VERSION.yaml

        # K8S SPANNERDEMO LOAD RESET
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{RESET_JOB_PARALLELISM}}/'$[gkenodes]'/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_RESET_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_RESET_YAMLTMPL-$instance-$db-$VERSION.yaml 

        printf "Creating Spanner Schema..."
        cd dataloadgenerator 
        echo -e "GOOGLE_APPLICATION_CREDENTIALS=/key.json\n"\
             "PROJECT=$project\n"\
             "INSTANCE=$instance\n"\
             "DATABASE=$db\n"\
             "DONTASK=true\n"\
             "CONFIGFILE=config.json" > config.env

        set -o xtrace

        docker run -v $PWD/../$instance-$db-$sconfig-key.json:/key.json -v $PWD/config.json:/config.json -v $PWD/schema/ticketshop-schema.sql:/schema.sql --env-file config.env -it spannerdemo-dataloadgenerator:$VERSION create schema.sql
        cd ..

        printf "Submit K8S configs to GKE cluster...\n"
        printf "You now can leave, and have coffee/breakfast/lunch/dinner and come back in hours/days depending on which tier you selected :)"
        # gcloud container clusters get-credentials $instance-$db-load-cluster --zone $zone
        # kubectl delete job ticketdatagen-tickets
        # kubectl delete job ticketdatagen-venues
        kubectl --context $CLUSTER_PREFIX$primary_region-01 apply -f $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_SECRET_YAMLTMPL-$instance-$db-$VERSION.yaml
        kubectl --context $CLUSTER_PREFIX$primary_region-01 apply -f $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_DATALOADCONFIG_YAMLTMPL-$instance-$db-$VERSION.yaml
        kubectl --context $CLUSTER_PREFIX$primary_region-01 apply -f $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_VENUESLOAD_YAMLTMPL-$instance-$db-$VERSION.yaml
        set +o xtrace
        # check for job to be finished in loop with timeout
        while [ 1 ]; do
            kubectlout=$(kubectl --context $CLUSTER_PREFIX$primary_region-01 get job spannerdemo-venuesload)

            regex="spannerdemo-venuesload[[:space:]]+([0-9]*)[[:space:]]+([0-9]*)"
            if [[ $kubectlout =~ $regex ]];
            then
                echo "checking if venues load job is finished..."
                if [[ ${BASH_REMATCH[1]} == ${BASH_REMATCH[2]} ]];
                then
                echo "Venues load job finished"
                break
                fi
            fi
            echo "waiting 5 sec before next check..."
            sleep 5
        done
        set -o xtrace
        # create tickets
        kubectl --context $CLUSTER_PREFIX$primary_region-01 create -f $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_TICKETSLOAD_YAMLTMPL-$instance-$db-$VERSION.yaml
        set +o xtrace
        # optional check for tickets to be finished
        while [ 1 ]; do
            kubectlout=$(kubectl --context $CLUSTER_PREFIX$primary_region-01 get job spannerdemo-ticketsload)

            regex="spannerdemo-ticketsload[[:space:]]+([0-9]*)[[:space:]]+([0-9]*)"
            if [[ $kubectlout =~ $regex ]];
            then
                echo "checking if ticket load job is finished..."
                if [[ ${BASH_REMATCH[1]} == ${BASH_REMATCH[2]} ]];
                then
                    echo "Tickets load job finished"
                    break
                fi
                echo "${BASH_REMATCH[2]} out of ${BASH_REMATCH[1]} finished"
            fi
            echo "waiting 1 minute before next check..."
            sleep 60
        done

        echo "Successful setup of the demo DB!"
        echo "Deploy influxdb, backend, dashboard and buybots ..."

        # K8S SPANNERDEMO BACKEND INFLUXDB
        sed -e 's/{{INFLUX_DATABASE}}/demo/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_BACKEND_INFLUX_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BACKEND_INFLUX_YAMLTMPL-$instance-$db-$VERSION.yaml

        # Deploy backend-influx and wait for external IP
        set -o xtrace
        kubectl --context $CLUSTER_PREFIX$primary_region-01 apply -f \
        $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BACKEND_INFLUX_YAMLTMPL-$instance-$db-$VERSION.yaml
        set +o xtrace

        echo "Waiting for external IP for spannerdemo-backend-influxdb service ..."
        iters=0
        influx_ip=""
        while [ -z $influx_ip  ] && (( iters <= 60 )); do
            sleep 10
            iters=$(($iters+1))
            influx_ip=$(kubectl --context $CLUSTER_PREFIX$primary_region-01 get svc spannerdemo-backend-influxdb --template="{{range .status.loadBalancer.ingress}}{{.ip}}{{end}}")
        done

        echo "External IP for spannerdemo-backend-influxdb service: $influx_ip"
        
        # K8S SPANNERDEMO BACKEND
        INFLUX_ADDR="http://$influx_ip:8086"

        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{SPANNER_INSTANCE}}/'$instance'/' -e 's/{{SPANNER_DATABASE}}/'$db'/' \
        -e 's/{{INFLUX_DATABASE}}/demo/' -e 's~{{INFLUX_ADDR}}~'$INFLUX_ADDR'~' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_BACKEND_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BACKEND_YAMLTMPL-$instance-$db-$VERSION.yaml 

        # K8S SPANNERDEMO BUYBOTS Europe
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{REGION}}/'europe'/' -e's/{{COUNTRIES}}/DE/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_BUYBOTS_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BUYBOTS_YAMLTMPL-europe-$instance-$db-$VERSION.yaml 

        # K8S SPANNERDEMO BUYBOTS ASIA
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{REGION}}/'asia'/' -e's/{{COUNTRIES}}/TW/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_BUYBOTS_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BUYBOTS_YAMLTMPL-asia-$instance-$db-$VERSION.yaml 

        # K8S SPANNERDEMO BUYBOTS US
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{REGION}}/'us'/' -e's/{{COUNTRIES}}/US/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_BUYBOTS_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BUYBOTS_YAMLTMPL-us-$instance-$db-$VERSION.yaml 

        #K8S SPANNERDEMO DASHBOARD
        sed -e 's/{{PROJECT}}/'$project'/' -e 's/{{VERSION}}/'$VERSION'/' \
        -e 's/{{INFLUX_DATABASE}}/demo/' -e 's~{{INFLUX_ADDR}}~'$INFLUX_ADDR'~' \
        -e 's/{{REFRESH_CYCLE_MS}}/100/' \
        <$K8S_CONFIGS_TMPLS_FOLDER$K8S_DASHBOARD_YAMLTMPL.yaml.template > $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_DASHBOARD_YAMLTMPL-$instance-$db-$VERSION.yaml 

        # Deploy dashboard
        kubectl --context $CLUSTER_PREFIX$primary_region-01 apply -f \
        $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_DASHBOARD_YAMLTMPL-$instance-$db-$VERSION.yaml

        set -o xtrace
        for region in "asia" "europe" "us"; do
            kubectl --context $CLUSTER_PREFIX$region-01 apply -f $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_SECRET_YAMLTMPL-$instance-$db-$VERSION.yaml

            kubectl --context $CLUSTER_PREFIX$region-01 apply -f \
            $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BACKEND_YAMLTMPL-$instance-$db-$VERSION.yaml 

            kubectl --context $CLUSTER_PREFIX$region-01 apply -f \
            $K8S_GENERATED_CONFIGS_FOLDER$DATE-$K8S_BUYBOTS_YAMLTMPL-$region-$instance-$db-$VERSION.yaml
        done
        set +o xtrace

        echo "Waiting for external IP for spannerdemo-dashboard service ..."
        iters=0
        dashboard_ip=""
        while [ -z $dashboard_ip  ] && (( iters <= 60 )); do
            sleep 10
            iters=$(($iters+1))
            dashboard_ip=$(kubectl --context $CLUSTER_PREFIX$primary_region-01 get svc spannerdemo-dashboard --template="{{range .status.loadBalancer.ingress}}{{.ip}}{{end}}")
        done

        echo "External IP for spannerdemo-dashboard: $dashboard_ip"
        echo "Checking how many tickets were generated ..."
        tickets=$(gcloud spanner databases execute-sql $db --instance $instance --sql "SELECT COUNT(*) FROM Ticket" | sed -n 2p)
        echo "Generated Tickets: $tickets"
        echo "Script execution successful, as far as I can tell ;) -- Enjoy!"
        ;;
    *)
        echo "Script execution aborted! cya :D"
        exit 0
        ;;
esac

