#!/bin/bash

mode=$1    # start, stop or bootstrap

STANDALONE_PID_FILE=/tmp/tmp-mysql.pid

case "$mode" in
  'start')
      echo "Starting MySQL"
      /var/vcap/packages/mariadb/bin/mysqld_safe &
      ;;

  'stop')
      echo "Stopping the cluster"
      /var/vcap/packages/mariadb/support-files/mysql.server stop
      ;;

  # This was mostly taken from /var/vcap/packages/mariadb/support-files/mysql.server
  # It cannot handle the fact the pid-file is in a different location than defined in my.cnf
  # We don't wait for the process to stop, because the waiting/polling takes places at a higher level.
  'stop-stand-alone')
      echo "Stopping the standalone node"
      mysqld_pid=`cat "$STANDALONE_PID_FILE"`

      if (kill -0 $mysqld_pid 2>/dev/null)
      then
        echo $echo_n "Shutting down node"
        kill $mysqld_pid
      else
        echo "Server process #$mysqld_pid is not running!"
        rm "$STANDALONE_PID_FILE"
      fi
      ;;

  'bootstrap')
      # Bootstrap the cluster, start the first node
      # that initiate the cluster
      echo "Bootstrapping the cluster"
      /var/vcap/packages/mariadb/bin/mysqld_safe --wsrep-new-cluster &
      ;;

  'stand-alone')
      echo "Starting the node in stand-alone mode"
      /var/vcap/packages/mariadb/bin/mysqld_safe --wsrep-on=OFF --wsrep-desync=ON --wsrep-OSU-method=RSU --wsrep-provider='none' --pid-file=$STANDALONE_PID_FILE &
      ;;
esac

exit 0
