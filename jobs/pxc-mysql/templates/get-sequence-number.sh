#!/usr/bin/env bash

source /var/vcap/packages/pxc-utils/pid_utils.sh

set -eu

# Nice output formatting
normal=$(tput sgr0)
bold=$(tput bold)
red=$(tput setaf 1)

pid=""
if [[ -f "/var/vcap/sys/run/pxc-mysql/mysql.pid" ]]; then
  pid=$(head -1 "/var/vcap/sys/run/pxc-mysql/mysql.pid")
fi

if [[ -n "${pid}" ]]; then
  echo ""
  echo -e "${bold}${red}\tCannot get sequence number while MySQL is running."
  echo -e "\tRefer to documentation on how to shut down running instances.${normal}"
  echo ""
  exit 1
fi

regex="[0-9]+$"

seq_no=$(cat /var/vcap/store/pxc-mysql/grastate.dat | grep 'seqno:')

if [[ "$seq_no" = "seqno:   -1" ]]; then
  GALERA_SIDECAR_ENDPOINT=$(grep -A 2 SidecarEndpoint /var/vcap/jobs/galera-agent/config/galera-agent-config.yaml)
  USERNAME="$(echo "$GALERA_SIDECAR_ENDPOINT" | grep Username | cut -d":" -f2 |  tr -d '[:space:]' )"
  PASSWORD="$(echo "$GALERA_SIDECAR_ENDPOINT" | grep Password | cut -d":" -f2 |  tr -d '[:space:]' )"

  set +e
  SEQUENCE_NUMBER=$((curl -f -S -s http://$USERNAME:$PASSWORD@localhost:9200/sequence_number) 2>&1)
  CURL_RESULT=$?
  set -e

  if  [[ $CURL_RESULT -ne 0 ]] ; then
    echo "${bold}${red}There was an error: "
    echo $SEQUENCE_NUMBER
    echo "${normal}"
  else
    seq_no=$SEQUENCE_NUMBER
  fi
fi

if [[ "$seq_no" =~ $regex ]]; then
  instance_id=$(cat /var/vcap/instance/id)
  json_output="{\"sequence_number\":${BASH_REMATCH},\"instance_id\":\"${instance_id}\"}";
fi

echo ${json_output}
