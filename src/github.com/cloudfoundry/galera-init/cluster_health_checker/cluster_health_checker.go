package cluster_health_checker

import (
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/lager/v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . UrlGetter
type UrlGetter interface {
	Get(url string) (*http.Response, error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . ClusterHealthChecker
type ClusterHealthChecker interface {
	HealthyCluster() bool
}

type httpClusterHealthChecker struct {
	clusterUrls []string
	logger      lager.Logger
	client      UrlGetter
}

func NewClusterHealthChecker(urls []string, logger lager.Logger, client UrlGetter) ClusterHealthChecker {
	return httpClusterHealthChecker{
		clusterUrls: urls,
		logger:      logger,
		client:      client,
	}
}

func (h httpClusterHealthChecker) HealthyCluster() bool {
	h.logger.Info("Checking for healthy cluster", lager.Data{
		"ClusterIps": h.clusterUrls,
	})
	for _, url := range h.clusterUrls {
		h.logger.Info("Checking if node is healthy: " + url)

		resp, err := h.client.Get(url)
		if err != nil {
			h.logger.Error("checking cluster member health failed", err)
			continue
		}
		if resp != nil && resp.StatusCode == 200 {
			h.logger.Info("node " + url + " is healthy - cluster is healthy.")
			return true
		}
		body, _ := ioutil.ReadAll(resp.Body)
		h.logger.Info("node "+url+" is NOT healthy", lager.Data{
			"status": resp.Status,
			"body":   string(body),
		})
	}

	h.logger.Info("No nodes in cluster are healthy.")
	return false
}
