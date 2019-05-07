# Configuration Guide

Security, Performance and General Configuration

There are a lot of properties available in pxc-release. Many are present only for specific use cases, are not generally important. We work hard to ensure that pxc-release ships with sane defaults, but provide many properties to tune and configure PXC for production. This purpose of this page is to explain how to best configure pxc-release.

This is a guide to help you understand which properties are worth your attention, limitations, and when some properties only make sense when changed as a set. This page can also call out the settings we tune when running in production.

## General

- For new installations, it's nice to set the name of the cluster.
- Unfortunately, in cluster mode, you cannot do this while upgrading; it will break the deploy.
- We recommend you make new_cluster_probe_timeout configurable (default 10s) https://www.pivotaltracker.com/story/show/145289667

## Security

- We recommend changing the default admin username from `root` to `admin` using the `cf_mysql.mysql.admin_username` property.
  - We've gotten customer feedback that using the login "root" is less secure.
  - When changing away from root, make sure to set `cf_mysql.mysql.previous_admin_username`, or you'll leave the root user with admin privs.

## Performance

- `engine_config.galera.wsrep_applier_threads` may configured to increase the number of Galera replication applier
  threads.  Values greater than 1 may improve improve replication performance for some workloads. For more information,
  see the
  [Percona XtraDB Cluster Documentation](https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-system-index.html#wsrep_slave_threads)
