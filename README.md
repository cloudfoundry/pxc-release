# pxc-release

Alpha Percona Xtradb Cluster release **Not ready for production use**

pxc-release is a BOSH release of MySQL Galera that can be used as a backing store for Cloudfoundry. The Galera Cluster Provider is [Percona Xtradb Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster). 
This release is intended as a drop-in replacement for [cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release).

<a name='components'></a>
# Components

<a name='mysql-server'></a>
## MySQL Server

<a name='proxy'></a>
## Proxy

Traffic to the MySQL cluster is routed through one or more proxy nodes. The current proxy implementation is [Switchboard](https://github.com/cloudfoundry-incubator/switchboard). This proxy acts as an intermediary between the client and the MySQL server, providing failover between MySQL nodes. The number of nodes is configured by the proxy job instance count in the deployment manifest.

**NOTE:** If the number of proxy nodes is set to zero, apps will be bound to the IP address of the first MySQL node in the cluster. If that IP address should change for any reason (e.g. loss of a VM) or a proxy was subsequently added, one would need to re-bind all apps to the IP address of the new node.

For more details see the [proxy documentation](/docs/proxy.md).

<a name='deploying'></a>
# Deploying
Instructions on deploying pxc-release with cf-deployment coming soon

<a name='migrating'></a>
# Migrating from cf-mysql-release
Instructions on migrating to pxc-release from cf-mysql-release coming soon

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
1. Check out `develop` of pxc-release
1. Create a feature branch (`git checkout -b <my_new_branch>`)
1. Make changes on your branch
1. Deploy your changes using pxc as the database for cf-deployment to your dev environment and run [CF Acceptance Tests (CATS)](https://github.com/cloudfoundry/cf-acceptance-tests/)
1. Push to your fork (`git push origin <my_new_branch>`) and submit a pull request

We favor pull requests with very small, single commits with a single purpose.

Your pull request is much more likely to be accepted if it is small and focused with a clear message that conveys the intent of your change.
