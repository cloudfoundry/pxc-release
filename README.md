# pxc-release

Percona Xtradb Cluster release

pxc-release is a BOSH release of MySQL Galera that can be used as a backing store for Cloudfoundry. The Galera Cluster
Provider is [Percona Xtradb Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster).

This bosh release deploys Percona XtraDB Cluster 8.0 by default, but may be configured to deploy Percona XtraDB Cluster 8.4.

<a name='deploying'></a>
# Deploying

## Deployment Topology

### Galera Clustered Mysql Topology (`pxc-mysql` job)
The `pxc-mysql` BOSH job runs mysql using Galera replication, across 1 or several nodes.

The typical clustered topology is 2 proxy nodes and 3 mysql nodes running the `pxc-mysql` BOSH job. The proxies can be separate vms or co-located with the pxc-mysql nodes.

You can also run this topology with a single mysql node running the `pxc-mysql` BOSH job and a single proxy job. In this case you would have a galera cluster of size 1, which does not provide high-availability.
#### Database nodes

The number of mysql nodes should always be odd, with a minimum count of three, to avoid [split-brain](http://en.wikipedia.org/wiki/Split-brain\_\(computing\)).
When a failed node comes back online, it will automatically rejoin the cluster and sync data from one of the healthy nodes.

#### Proxy nodes

Two proxy instances are recommended. The second proxy is intended to be used in a failover capacity. You can also choose to place a load balancer in front of both proxies, or use [BOSH DNS](https://bosh.io/docs/dns.html) to send traffic to both proxies.

In the event the first proxy fails, the second proxy will still be able to route requests to the mysql nodes.

The proxies both will route traffic to the lowest-indexed healthy galera node, according to the galera index (not bosh index).

Traffic to the MySQL cluster is routed through one or more proxy nodes. The current proxy implementation is [Switchboard](https://github.com/cloudfoundry-incubator/switchboard). This proxy acts as an intermediary between the client and the MySQL server, providing failover between MySQL nodes. The number of nodes is configured by the proxy job instance count in the deployment manifest.

**NOTE:** If the number of proxy nodes is set to zero, apps will be bound to the IP address of the first MySQL node in the cluster. If that IP address should change for any reason (e.g. loss of a VM) or a proxy was subsequently added, one would need to re-bind all apps to the IP address of the new node.

For more details see the [proxy documentation](/docs/proxy.md).

<a name='deploying-new-deployments'></a>
## Deploying
<a name='deploying-with-cf-deployment'></a>
### Deploying CF with pxc-release (using the clustered topology)
Use the [cf-deployment manifests](https://github.com/cloudfoundry/cf-deployment) with the `scale-database-cluster.yml` ops file.

<a name='deploying-clustered></a>
### Deploying pxc-release clustered

To deploy a clustered deployment, use the [pxc-deployment.yml manifest](pxc-deployment.yml) and apply the [use-clustered](operations/use-clustered.yml) opsfile:

```
bosh -d <deployment> deploy --ops-file operations/use-clustered.yml pxc-deployment.yml
```

<a name='deploying-standalone'></a>
### Deploying pxc-release standalone

To deploy a standalone deployment, use the [pxc-deployment.yml manifest](pxc-deployment.yml):

```bash
bosh -d <deployment> deploy pxc-deployment.yml
```

### Deploying pxc-release with Percona XtraDB Cluster 8.4 support

Percona XtraDB Cluster 8.0 will be [end-of-life as of April
2026](https://www.percona.com/services/policies/percona-software-support-lifecycle#lifecycle). You are encouraged to
upgrade to Percona XtraDB Cluster 8.4 as soon as possible.

For backwards compatibility, this release can still deploy Percona XtraDB Cluster 8.0 instances by setting the
`mysql_version` property of the `pxc-mysql` job to "8.0".

```bash
bosh -d <deployment> deploy pxc-deployment.yml -o operations/mysql-version.yml -v mysql-version="'8.0'"
```

Upgrades from a deployment using "mysql_version='8.0'" to a deployment using "mysql_version='8.4'" is supported.  You are
encourage to validate application compatibility and backing up your existing Percona XtraDB Cluster 8.0 data before
undertaking a major database upgrade to a production deployment.

**Important** Percona XtraDB Cluster 8.4 does not support in-place downgrades to Percona XtraDB Cluster 8.0.  If you
attempt such a downgrade, the deployment will fail on the first node with an error in the MySQL error log.

<a name='contribution-guide'></a>
# Contribution Guide

The Cloud Foundry team uses GitHub and accepts contributions via
[pull request](https://help.github.com/articles/using-pull-requests).

## Contributor License Agreement

Follow these steps to make a contribution to any of our open source repositories:

1. Ensure that you have completed our CLA Agreement for
  [individuals](https://www.cloudfoundry.org/pdfs/CFF_Individual_CLA.pdf) or
  [corporations](https://www.cloudfoundry.org/pdfs/CFF_Corporate_CLA.pdf).

1. Set your name and email (these should match the information on your submitted CLA)

        git config --global user.name "Firstname Lastname"
        git config --global user.email "your_email@example.com"

## General Workflow

1. Fork the repository
1. Check out the `main` branch of pxc-release
1. Create a feature branch (`git checkout -b <my_new_branch>`)
1. Make changes on your branch
1. Deploy your changes using pxc as the database for cf-deployment to your dev environment and run [CF Acceptance Tests (CATS)](https://github.com/cloudfoundry/cf-acceptance-tests/)
1. Push to your fork (`git push origin <my_new_branch>`) and submit a pull request

We favor pull requests with very small, single commits with a single purpose.

Your pull request is much more likely to be accepted if it is small and focused with a clear message that conveys the intent of your change.
