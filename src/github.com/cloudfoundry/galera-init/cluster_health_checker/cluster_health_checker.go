package cluster_health_checker

import (
	"net/http"
	"strings"

	"github.com/cloudfoundry/mariadb_ctrl/logger"
)

var MakeRequest = http.Get

type ClusterHealthChecker interface {
	HealthyCluster() bool
}

type httpClusterHealthChecker struct {
	clusterIps []string
	logger     logger.Logger
}

func NewClusterHealthChecker(ips string, logger logger.Logger) ClusterHealthChecker {
	return httpClusterHealthChecker{
		clusterIps: strings.Split(ips, ","),
		logger:     logger,
	}
}

func (h httpClusterHealthChecker) HealthyCluster() bool {
	for _, ip := range h.clusterIps {
		h.logger.Log("Checking if node is healthy: " + ip)

		resp, _ := MakeRequest("http://" + ip + ":9200/")
		if resp != nil && resp.StatusCode == 200 {
			h.logger.Log("node " + ip + " is healthy - cluster is healthy.")
			return true
		}
	}

	h.logger.Log("No nodes in cluster are healthy.")
	return false
}
