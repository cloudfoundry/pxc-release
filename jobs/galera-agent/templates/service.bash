#!/usr/bin/env bash

set -o nounset

if [[ -f /var/vcap/bosh/etc/monit-access-helper.sh ]]; then
  # Ubuntu-jammy stemcells provide a monit access helper
  source /var/vcap/bosh/etc/monit-access-helper.sh
  permit_monit_access
elif [[ -x /var/vcap/bosh/etc/bosh-enable-monit-access ]]; then
  # Ubuntu-noble stemcell v1.267 and later require calling an explicit bosh command
  /var/vcap/bosh/etc/bosh-enable-monit-access
elif type -f -p nft >/dev/null && nft list ruleset | grep -q monit_output; then
  # Ubuntu-noble prior to v1.267 require direct nft chain manipulation
  rule_handle=$(nft -a list ruleset | awk '/galera-agent/ { print $NF }')
  if [[ -n $rule_handle ]]; then
    nft delete rule inet filter monit_output handle "${rule_handle}"
  fi
  nft insert rule inet filter monit_output index 2 \
    socket cgroupv2 level 2 "system.slice/runc-bpm-galera-agent.scope" \
    ip daddr 127.0.0.1 tcp dport 2822 \
    log prefix '"Matched cgroup galera-agent monit access rule: "' \
    accept
fi

# unmount fails under newer Ubuntu kernels without using the "--make-rslave" option
# This affects the ubuntu-jammy stemcell 1.351 .. 1.390
mount --make-rslave /sys/fs/cgroup
umount --recursive /sys/fs/cgroup
umount /var/vcap/bosh/etc
exec chpst -u vcap -- /var/vcap/packages/galera-agent/bin/galera-agent \
  --configPath=/var/vcap/jobs/galera-agent/config/galera-agent-config.yml \
  --timeFormat="<%= p('logging.format.timestamp') %>"
