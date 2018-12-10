#!/usr/bin/env bash

set -euo pipefail

: "${WSREP_CLUSTER_ADDRESS:?WSREP_CLUSTER_ADDRESS must be set}"
: "${WSREP_NODE_ADDRESS:?WSREP_NODE_ADDRESS must be set}"
: "${WSREP_NODE_NAME:?WSREP_NODE_NAME must be set}"

function render_mysql_config {
    sed -e "s^@@WSREP_CLUSTER_ADDRESS@@^${WSREP_CLUSTER_ADDRESS}^" \
        -e "s^@@WSREP_NODE_ADDRESS@@^${WSREP_NODE_ADDRESS}^" \
        -e "s^@@WSREP_NODE_NAME@@^${WSREP_NODE_NAME}^" \
        /usr/local/etc/my.cnf.template > /var/vcap/jobs/pxc-mysql/config/my.cnf
}

function initialize_mysql_datadir {
    mysqld --initialize-insecure \
           --disable-log-error \
           --init-file=/usr/local/etc/init.sql
}

function apply_initial_cluster_state {
    if [[ -n "${INITIAL_CLUSTER_STATE:-}" ]]; then
        echo -n "${INITIAL_CLUSTER_STATE}" \
            > "${CLUSTER_STATE_FILE:-/var/lib/mysql/node_state.txt}}"
    fi
}

function setup() {
    render_mysql_config
    initialize_mysql_datadir
    apply_initial_cluster_state
}

setup

exec galera-init "$@"
