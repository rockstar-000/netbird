#!/bin/bash

source setup.env

if [[ "x-$WIRETRUSTEE_DOMAIN" == "x-" ]]
then
  echo WIRETRUSTEE_DOMAIN is not set, please update your setup.env file
  exit 1
fi

# local development or tests
if [[ $WIRETRUSTEE_DOMAIN == "localhost" || $WIRETRUSTEE_DOMAIN == "127.0.0.1" ]]
then
  export WIRETRUSTEE_MGMT_API_ENDPOINT=http://$WIRETRUSTEE_DOMAIN:$WIRETRUSTEE_MGMT_API_PORT
  unset WIRETRUSTEE_MGMT_API_CERT_FILE
  unset WIRETRUSTEE_MGMT_API_CERT_KEY_FILE
fi

# if not provided, we generate a turn password
if [[ "x-$TURN_PASSWORD" == "x-" ]]
then
  export TURN_PASSWORD=$(openssl rand -base64 32|sed 's/=//g')
fi

# if wiretrustee-mgmt already exists, leave it, else create new mgmt with netbird
OLD_PREFIX='wiretrustee-'
if docker volume ls | grep -q "${OLD_PREFIX}mgmt"; then 
    VOLUME_PREFIX=$OLD_PREFIX
fi

export MGMT_VOLUMENAME="${$VOLUME_PREFIX}mgmt"
export LETSENCRYPT_VOLUMENAME="${$VOLUME_PREFIX}letsencrypt"

envsubst < docker-compose.yml.tmpl > docker-compose.yml
envsubst < management.json.tmpl > management.json
envsubst < turnserver.conf.tmpl > turnserver.conf
