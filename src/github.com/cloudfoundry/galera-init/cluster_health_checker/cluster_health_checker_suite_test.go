package cluster_health_checker_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterHealthChecker(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Cluster Health Checker Suite")
}
