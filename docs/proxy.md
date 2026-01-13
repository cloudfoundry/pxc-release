# Proxy

In pxc-release, [Switchboard](../src/github.com/cloudfoundry-incubator/switchboard) is used to proxy TCP connections to healthy Percona XtraDB Cluster nodes.

A proxy is used to gracefully handle failure of Percona XtraDB Cluster nodes. Use of a proxy permits very fast, unambiguous failover to other nodes within the cluster in the event of a node failure.

When a node becomes unhealthy, the proxy re-routes all subsequent connections to a healthy node. All existing connections to the unhealthy node are closed.

## Consistent Routing

At any given time, each deployed proxy will only route to one active node. The proxy will select the node with the lowest `wsrep_local_index`. The `wsrep_local_index` is a Galera status variable indicating Galera's internal indexing of nodes. The index can change, and there is no guarantee that it corresponds to the BOSH index. The chosen active node will continue to be the only active node until it becomes unhealthy.

If multiple proxies are used in parallel (ex: behind a load-balancer) the proxies behave independently with no proxy to proxy coordination. 
However, the logic to choose a node is identical in each proxy therefore the proxies will route connections to the same active Cluster node. 

## Node Health

### Healthy

The proxy queries an HTTP healthcheck process, co-located on the database node, when determining where to route traffic. 

If the healthcheck process returns HTTP status code of 200, the node is added to the pool of healthy nodes.

### Unhealthy

If the node healthcheck returns HTTP status code 503, the node is considered unhealthy.

This happens when a node becomes non-primary, as specified by the [cluster-behavior docs](cluster-behavior.md).

The proxy will sever all existing connections to newly unhealthy nodes. Clients are expected to handle reconnecting on connection failure. The proxy will route new connections to a healthy node, assuming such a node exists.

### Unresponsive

If node health cannot be determined due to an unreachable or unresponsive healthcheck endpoint, the proxy will consider the node unhealthy. This may happen if there is a network partition or if the VM containing the healthcheck and Percona XtraDB Cluster node died.

## Proxy Health

### Healthy

The proxy deploys with a bosh-dns healthcheck validating the proxy's communication with
its targeted mysql node. If this healthcheck fails, then bosh-dns removes the proxy instance from its healthy pool of instances. More information is in [BOSH Native DNS Support](https://bosh.io/docs/dns/#healthiness) documentation.

### Unhealthy
If the proxy becomes unresponsive for any reason the other deployed proxies are able to accept all client connections.

## State Snapshot Transfer (SST)

When a new node is added to the cluster or rejoins the cluster, it receives state from the primary component via a process called SST. A single "donor node" from the primary component is chosen to provide state to the new node. pxc-release is configured to transfer state via [Xtrabackup](https://docs.percona.com/percona-xtradb-cluster/8.4/state-snapshot-transfer.html#use-percona-xtrabackup). Xtrabackup lets the donor node continue accepting reads and writes while concurrently providing its state to the new node.

## Proxy count

If the operator sets the total number of proxies to 0 hosts in their manifest, then applications will start routing connections directly to one healthy Percona XtraDB Cluster node making that node a single point of failure for the cluster.

The recommended number of proxies is 2; this provides redundancy should one of the proxies fail.

## Setting a load balancer in front of the proxies

The proxy tier is responsible for routing connections from applications to healthy Percona XtraDB Cluster nodes, even in the event of node failure.

Multiple proxies are recommended for uninterrupted operation. Using a load balancer in front of the proxies ensures distributed connection requests and minimal disruption in the event of the unavailability of any proxy.

Configure a load balancer<sup>[[2]](#configuring-load-balancer)</sup> to route client connections to all proxy IPs, and configure the MySQL service<sup>[[3]](#configuring-pxc-release-to-give-applications-the-address-of-the-load-balancer)</sup> to give bound applications a hostname or IP address that resolves to the load balancer.

### Configuring load balancer

Configure the load balancer to route traffic for TCP port 3306 to the IPs of all proxy instances on TCP port 3306.

Next, configure the load balancer's healthcheck to use the proxy health port.
The proxies have an HTTP server listening on the health port. It returns 200 in all cases and for all endpoints. This can be used to configure a Load Balancer that requires HTTP healthchecks.

Because HTTP uses TCP connections, the port also accepts TCP requests, useful for configuring a Load Balancer with a TCP healthcheck.

By default, the health port is 1936 to maintain backwards compatibility with previous releases, but this port can be configured by adding the `cf_mysql.proxy.health_port` manifest property to the proxy job and deploying.

Use the operations file operations/add-proxy-load-balancer.yml to add a load balancer to the proxies.

## API

The proxy hosts a JSON API at `<bosh job index>-proxy-p-mysql.<system domain>/v0/`.

The API provides the following route:

Request:
*  Method: GET
*  Path: `/v0/backends`
*  Params: ~
*  Headers: Basic Auth

Response:

```json
[
  {
    "host": "10.10.0.13",
    "port": 3306,
    "healthy": true,
    "name": "mysql/26bf09bb-1c65-4de3-8b37-3138adf88e93",
    "currentSessionCount": 0,
    "active": false,
    "trafficEnabled": true
  },
  {
    "host": "10.10.0.14",
    "port": 3306,
    "healthy": true,
    "name": "mysql/1b5d0dba-e5b7-4c13-9b05-8c8c493dc7af",
    "currentSessionCount": 0,
    "active": true,
    "trafficEnabled": true
  },
  {
    "host": "10.10.0.15",
    "port": 3306,
    "healthy": true,
    "name": "mysql/7964d5c3-c2a5-4be1-9493-9cf06ec2ac46",
    "currentSessionCount": 0,
    "active": false,
    "trafficEnabled": true
  }
]
```

## Dashboard

The proxy also provides a Dashboard UI to view the current status of the database nodes. This is hosted at `<bosh job index>-proxy-p-mysql.<system domain>`.

The Proxy Springboard page at `proxy-p-mysql.<system domain>` contains links to each of the Proxy Dashboard pages.
