#!/bin/bash

mode=$1    # start, stop or bootstrap

case "$mode" in
  'start')
    echo "Starting MySQL"
    /var/vcap/packages/mariadb/bin/mysqld_safe &
    ;;

  'stop')
    echo "Stopping the cluster"
    /var/vcap/packages/mariadb/support-files/mysql.server stop
    ;;

  'bootstrap')
      # Bootstrap the cluster, start the first node
      # that initiate the cluster
      echo "Bootstrapping the cluster"
      /var/vcap/packages/mariadb/bin/mysqld_safe --wsrep-new-cluster &
      ;;
esac

exit 0
