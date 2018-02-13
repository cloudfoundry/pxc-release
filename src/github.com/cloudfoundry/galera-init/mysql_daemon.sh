#!/bin/bash

set -e

mode=$1

case "$mode" in
  'stop')
      echo "Stopping the cluster"
      /var/vcap/packages/pxc/bin/mysqladmin --defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf shutdown
      ;;

  'bootstrap')
      # Bootstrap the cluster, start the first node
      # that initiate the cluster
      echo "Bootstrapping the cluster"
      /var/vcap/packages/pxc/bin/mysqld_safe --wsrep-new-cluster &
      ;;

  'stand-alone')
      echo "Starting the node in stand-alone mode"
      /var/vcap/packages/pxc/bin/mysqld --wsrep-on=OFF --wsrep-desync=ON --wsrep-OSU-method=RSU --wsrep-provider='none' --skip-networking --daemonize
      ;;

  'status')
      echo "Getting status of mysql process (exit 0 == running)"
      /var/vcap/packages/pxc/bin/mysqladmin --defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf status
      ;;
esac

exit 0
