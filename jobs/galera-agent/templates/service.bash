#!/usr/bin/env bash

set -o nounset

# Setup firewall rule to allow monit access from this job.
# Try new nftables firewall approach first (bosh-agent with monit_access_jobs chain).
if /var/vcap/packages/bosh-monit-access/bin/bosh-monit-access --check; then
  # New firewall with jobs chain exists - use bosh-monit-access helper
  /var/vcap/packages/bosh-monit-access/bin/bosh-monit-access
else
  # Fallback to old approaches for backward compatibility with older stemcells
  if type -f -p nft >/dev/null && nft list ruleset | grep -q monit_output; then
    rule_handle=$(nft -a list ruleset | awk '/galera-agent/ { print $NF }')
    if [[ -n $rule_handle ]]; then
      nft delete rule inet filter monit_output handle "${rule_handle}"
    fi
    nft insert rule inet filter monit_output index 2 \
      socket cgroupv2 level 2 "system.slice/runc-bpm-galera-agent.scope" \
      ip daddr 127.0.0.1 tcp dport 2822 \
      log prefix '"Matched cgroup galera-agent monit access rule: "' \
      accept
  elif [[ -f /var/vcap/bosh/etc/monit-access-helper.sh ]]; then
    source /var/vcap/bosh/etc/monit-access-helper.sh
    permit_monit_access
  fi
fi

# unmount fails under newer Ubuntu kernels without using the "--make-rslave" option
# This affects the ubuntu-jammy stemcell 1.351 .. 1.390
mount --make-rslave /sys/fs/cgroup
umount --recursive /sys/fs/cgroup
umount /var/vcap/bosh/etc
exec chpst -u vcap -- /var/vcap/packages/galera-agent/bin/galera-agent \
  --configPath=/var/vcap/jobs/galera-agent/config/galera-agent-config.yml \
  --timeFormat="<%= p('logging.format.timestamp') %>"
