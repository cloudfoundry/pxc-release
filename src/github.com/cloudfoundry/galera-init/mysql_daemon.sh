#!/bin/bash

set -e

mode=$1

case "$mode" in
  'stop')
      echo "Stopping the cluster"
      /var/vcap/packages/mariadb/support-files/mysql.server stop > /dev/null 2>&1  &
      ;;

  'bootstrap')
      # Bootstrap the cluster, start the first node
      # that initiate the cluster
      echo "Bootstrapping the cluster"
      /var/vcap/packages/mariadb/bin/mysqld_safe --wsrep-new-cluster &
      ;;

  'stand-alone')
      echo "Starting the node in stand-alone mode"
      /var/vcap/packages/mariadb/bin/mysqld_safe --wsrep-on=OFF --wsrep-desync=ON --wsrep-OSU-method=RSU --wsrep-provider='none' &
      ;;

  'status')
      echo "Getting status of mysql process (exit 0 == running)"
      /var/vcap/packages/mariadb/support-files/mysql.server status
      ;;
esac

exit 0
