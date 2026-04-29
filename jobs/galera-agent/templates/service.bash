#!/usr/bin/env bash

set -o nounset

# unmount fails under newer Ubuntu kernels without using the "--make-rslave" option
# This affects the ubuntu-jammy stemcell 1.351 .. 1.390
mount --make-rslave /sys/fs/cgroup
umount --recursive /sys/fs/cgroup
exec chpst -u vcap -- /var/vcap/packages/galera-agent/bin/galera-agent \
  --configPath=/var/vcap/jobs/galera-agent/config/galera-agent-config.yml \
  --timeFormat="<%= p('logging.format.timestamp') %>"
