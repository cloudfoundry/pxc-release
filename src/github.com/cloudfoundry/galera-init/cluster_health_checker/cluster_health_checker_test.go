package cluster_health_checker_test

import (
	"errors"
	"net/http"

	. "github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterHealthChecker.HealthyCluster()", func() {
	var testLogger lagertest.TestLogger
	BeforeEach(func() {
		testLogger = *lagertest.NewTestLogger("cluster_health_checker")
	})

	It("Returns true when a reachable node returns healthy", func() {
		requestUrls := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestUrls = append(requestUrls, url)
			return &http.Response{StatusCode: 200}, nil
		}

		checker := NewClusterHealthChecker("1.2.3.4,5.6.7.8", testLogger)

		Expect(checker.HealthyCluster()).To(BeTrue())
		Expect(requestUrls).To(Equal([]string{"http://1.2.3.4:9200/"}))
	})

	It("Returns false when all nodes are reachable and return unhealthy", func() {
		requestUrls := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestUrls = append(requestUrls, url)
			return &http.Response{StatusCode: 503}, nil
		}

		checker := NewClusterHealthChecker("1.2.3.4,5.6.7.8", testLogger)

		Expect(checker.HealthyCluster()).To(BeFalse())
		Expect(requestUrls).To(Equal([]string{"http://1.2.3.4:9200/", "http://5.6.7.8:9200/"}))
	})

	It("Returns false when all nodes are not reachable", func() {
		requestUrls := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestUrls = append(requestUrls, url)
			return nil, errors.New("Timed out")
		}

		checker := NewClusterHealthChecker("1.2.3.4,5.6.7.8", testLogger)

		Expect(checker.HealthyCluster()).To(BeFalse())
		Expect(requestUrls).To(Equal([]string{"http://1.2.3.4:9200/", "http://5.6.7.8:9200/"}))
	})
})
