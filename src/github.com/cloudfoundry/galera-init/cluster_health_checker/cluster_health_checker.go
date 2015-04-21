package cluster_health_checker

import (
	"net/http"

	"github.com/pivotal-golang/lager"
)

var MakeRequest = http.Get

type ClusterHealthChecker interface {
	HealthyCluster() bool
}

type httpClusterHealthChecker struct {
	clusterIps []string
	logger     lager.Logger
}

func NewClusterHealthChecker(ips []string, logger lager.Logger) ClusterHealthChecker {
	return httpClusterHealthChecker{
		clusterIps: ips,
		logger:     logger,
	}
}

func (h httpClusterHealthChecker) HealthyCluster() bool {
	h.logger.Info("Checking for healthy cluster", lager.Data{
		"ClusterIps": h.clusterIps,
	})
	for _, ip := range h.clusterIps {
		h.logger.Info("Checking if node is healthy: " + ip)

		resp, _ := MakeRequest("http://" + ip + ":9200/")
		if resp != nil && resp.StatusCode == 200 {
			h.logger.Info("node " + ip + " is healthy - cluster is healthy.")
			return true
		}
	}

	h.logger.Info("No nodes in cluster are healthy.")
	return false
}
