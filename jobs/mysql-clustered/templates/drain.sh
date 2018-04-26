#!/bin/bash -e

/var/vcap/packages/pxc/bin/mysqladmin --defaults-file=/var/vcap/jobs/mysql-clustered/config/mylogin.cnf shutdown > /dev/null
return_code=$?
echo 0
exit ${return_code}
