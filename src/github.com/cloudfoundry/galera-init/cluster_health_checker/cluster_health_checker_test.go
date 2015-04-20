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

	It("Constructs the correct url", func() {
		requestURLs := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 200}, nil
		}

		checker := NewClusterHealthChecker([]string{"1.2.3.4"}, testLogger)
		checker.HealthyCluster()

		Expect(requestURLs).To(Equal([]string{"http://1.2.3.4:9200/"}))

	})

	It("Immediately returns true when a reachable node returns healthy", func() {
		requestURLs := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 200}, nil
		}

		checker := NewClusterHealthChecker([]string{"1.2.3.4", "5.6.7.8"}, testLogger)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeTrue())

		Expect(len(requestURLs)).To(Equal(1))
		Expect(requestURLs[0]).To(ContainSubstring("1.2.3.4"))
	})

	It("Returns false when all nodes are reachable and return unhealthy", func() {
		requestURLs := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 503}, nil
		}

		checker := NewClusterHealthChecker([]string{"1.2.3.4", "5.6.7.8"}, testLogger)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeFalse())
		Expect(len(requestURLs)).To(Equal(2))
	})

	It("Returns false when all nodes are not reachable", func() {
		requestURLs := []string{}
		MakeRequest = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return nil, errors.New("Timed out")
		}

		checker := NewClusterHealthChecker([]string{"1.2.3.4", "5.6.7.8"}, testLogger)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeFalse())
		Expect(len(requestURLs)).To(Equal(2))
	})
})
