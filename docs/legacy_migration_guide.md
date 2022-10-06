## cf-mysql-release has been deprecated since Aug 5, 2019 and is no longer supported by cf-deployment

<a name='migrating-with-cf-deployment'></a>
## Migrating from cf-mysql-release

Requirements:

[cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release/) v36.12.0 or greater

<a name='migrating-with-cf-deployment'></a>
### Migrating CF with pxc-release
Use the [cf-deployment manifests](https://github.com/cloudfoundry/cf-deployment) with the `migrate-cf-mysql-to-pxc.yml` ops file. It is advisable to take a backup first.
- ⚠️ `migrate-cf-mysql-to-pxc.yml` will scale down a cluster to a single node. This is required for migration. Be sure to re-set to the appropriate number of instances when switching to `use-pxc.yml` subsequently.

The ops file will trigger the same migration procedure described in [Using PXC release with other deployments](#migrating-with-non-cf-deployments)
- If your cf-deployment uses CredHub, be sure to also include the [secure-service-credentials-with-pxc-release.yml](https://github.com/cloudfoundry/cf-deployment/blob/master/operations/experimental/secure-service-credentials-with-pxc-release.yml) ops file.

After migrating, use the [Deploying CF with pxc-release](#deploying-with-cf-deployment) docs for your next deploy.

<a name='migrating-with-non-cf-deployments'></a>
### Using PXC release with other deployments

1. Make backups according to your usual backup procedure.
1. Get the latest pxc bosh release from [bosh.io](http://bosh.io/releases/github.com/cloudfoundry-incubator/pxc-release)
2. Add the release to your manifest
2. ⚠️ **Scale down to 1 node and ensure the persistent disk has enough free space to double the size of the mysql data.**
3. Make the following changes to your bosh manifest:
    * Add the `pxc-mysql` job from `pxc-release` to the instance group that has the `mysql` job from `cf-mysql-release`
    * Configure the `pxc-mysql` job with the same credentials and property values as the `mysql` job
    * To run the migration:
        * Set the `cf_mysql_enabled: false` property on the `mysql` job
        * Set the `pxc_enabled: true` property on `pxc-mysql` job
        * Switch the proxies to use the proxy job from `pxc-release` instead of `cf-mysql-release`
        * Deploy using BOSH

    * To prepare for the migration, but not run it immediately:
        * Set the `cf_mysql_enabled: true` property on the `mysql` job
        * Set the `pxc_enabled: false` property on `pxc-mysql` job
        * Deploy using BOSH
        * The MySQL will run as normal with only the `cf-mysql-release` running
        * In order to trigger the migration, redeploy with `cf_mysql_enabled: false` and `pxc_enabled: true`

    * ⚠️ **Do not enable both releases or disable both releases. Only enable one at a time.**
4. The migration is triggered by deploying with `cf_mysql_enabled: false` and `pxc_enabled: true`. The `pre-start` script for the `pxc-mysql` job in `pxc-release` starts both the Mariadb MySQL from the `cf-mysql-release` and the Percona MySQL from `pxc-release`. The migration dumps the MariaDB MySQL and loads that data into the Percona MySQL. This is done using pipes, so the dump is not written to disk, in order to reduce the use of disk space. The MariaDB MySQL is then stopped, leaving only the Percona MySQL running.
    * ⚠️ **MySQL DB will experience downtime during the migration**
5. After the migration, you can optionally clean up your deployment:
    * The migration will make a copy of the MySQL data on the persistent disk. To reduce disk usage, you can delete the old copy of the data in `/var/vcap/store/mysql` after you feel comfortable in the success of your migration. Do **NOT** delete the new copy of the data in `/var/vcap/store/pxc-mysql`.
    * Deploy only the `pxc-release` and not the `cf-mysql-release` in future deployments per [Deploying new deployments](#deploying-new-deployments), to free up disk space used by the `cf-mysql-release`.

6. Scale back up to the recommended 3 nodes, if desired.

## Notes

* As of pxc 0.15.x, we implemented bpm support in the pxc-mysql job. bpm puts a hard time limit on monit stop operations
  and will eventually SIGKILL all processes in the bpm container if mysql takes too long to shut down.
  When pxc-release is deployed in a Galera topology, this will cause the node to reinitialize via an SST operation. During
  SST a node will remove its local data directory and replace it with data provided by another member of the cluster.
