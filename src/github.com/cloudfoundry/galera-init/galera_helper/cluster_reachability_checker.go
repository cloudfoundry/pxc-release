package galera_helper

import (
	"github.com/cloudfoundry/mariadb_ctrl/logger"
	"net/http"
	"strings"
)

var MakeRequest = http.Get

type ClusterReachabilityChecker interface {
	AnyNodesReachable() bool
}

type httpClusterReachabilityChecker struct {
	clusterIps []string
	logger     logger.Logger
}

func NewClusterReachabilityChecker(ips string, logger logger.Logger) ClusterReachabilityChecker {
	return httpClusterReachabilityChecker{
		clusterIps: strings.Split(ips, ","),
		logger:     logger,
	}
}

func (h httpClusterReachabilityChecker) AnyNodesReachable() bool {
	for _, ip := range h.clusterIps {
		h.logger.Log("Checking if node is reachable: " + ip)

		resp, _ := MakeRequest("http://" + ip + ":9200/")
		if resp != nil && resp.StatusCode == 200 {
			h.logger.Log("At least one node in cluster is reachable.")
			return true
		}
	}

	h.logger.Log("No nodes in cluster are reachable.")
	return false
}
