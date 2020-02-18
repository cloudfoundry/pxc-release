#!/usr/bin/env bash
set -eux

<% if link('mysql').p('pxc_enabled') == true %>
/var/vcap/packages/pxc/bin/mysql \
  --defaults-file="/var/vcap/jobs/pxc-mysql/config/mylogin.cnf" \
  < "/var/vcap/jobs/galera-agent/config/galera-agent-setup.sql"
<% end %>
