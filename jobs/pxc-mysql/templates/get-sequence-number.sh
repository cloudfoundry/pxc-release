#!/usr/bin/env bash

source /var/vcap/packages/cf-mysql-common/pid_utils.sh

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
  /var/vcap/packages/pxc/bin/mysqld --defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf --wsrep-recover
  seq_no=$(grep "Recovered position" /var/vcap/sys/log/pxc-mysql/mysql.err.log | tail -1)
fi

if [[ "$seq_no" =~ $regex ]]; then
  instance_id=$(cat /var/vcap/instance/id)
  json_output="{\"sequence_number\":${BASH_REMATCH},\"instance_id\":\"${instance_id}\"}";
fi

echo ${json_output}
