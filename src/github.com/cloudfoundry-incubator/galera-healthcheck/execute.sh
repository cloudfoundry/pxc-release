#!/bin/bash

MysqlUser=$1
MysqlPassword=$2
HAProxyIp=$3

echo "Mysql user: $1"
echo "HAProxy IP: $3"

MysqlPath=/var/vcap/packages/mariadb/bin/mysql

# This does not currently work on bosh lite, due to warden messing up the HAProxy ip.
# It will still log that it is attempting to chop things, but won't actually do anything.
for id in `$MysqlPath -u $MysqlUser -p$MysqlPassword -e "show processlist;" | grep $HAProxyIp | awk '{print $1}'`;
do echo "going to kill connection $id"; $MysqlPath -u $MysqlUser -p$MysqlPassword -e "kill $id";
done
