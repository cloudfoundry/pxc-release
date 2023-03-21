package cluster_health_checker_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("ClusterHealthChecker.HealthyCluster()", func() {
	var testLogger lagertest.TestLogger
	var fakeUrlGetter *cluster_health_checkerfakes.FakeUrlGetter

	BeforeEach(func() {
		fakeUrlGetter = &cluster_health_checkerfakes.FakeUrlGetter{}
		testLogger = *lagertest.NewTestLogger("cluster_health_checker")
	})

	It("Immediately returns true when a reachable node returns healthy", func() {
		requestURLs := []string{}
		fakeUrlGetter.GetStub = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 200}, nil
		}

		checker := NewClusterHealthChecker([]string{"https://1.2.3.4:9201", "https://5.6.7.8:9201"}, testLogger, fakeUrlGetter)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeTrue())

		Expect(len(requestURLs)).To(Equal(1))
		Expect(requestURLs[0]).To(ContainSubstring("1.2.3.4"))
	})

	It("Returns false when all nodes are reachable and return unhealthy", func() {
		requestURLs := []string{}
		fakeUrlGetter.GetStub = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: io.NopCloser(bytes.NewBufferString("some body"))}, nil
		}

		checker := NewClusterHealthChecker([]string{"https://1.2.3.4:9201", "https://5.6.7.8:9201"}, testLogger, fakeUrlGetter)
		healthy := checker.HealthyCluster()

		Expect(testLogger.LogMessages()).To(ContainElement("cluster_health_checker.node https://1.2.3.4:9201 is NOT healthy"))
		Expect(testLogger.LogMessages()).To(ContainElement("cluster_health_checker.node https://5.6.7.8:9201 is NOT healthy"))
		Expect(testLogger.Buffer()).To(gbytes.Say(`"status":"503 Service Unavailable"`))
		Expect(testLogger.Buffer()).To(gbytes.Say(`"body":"some body"`))

		Expect(healthy).To(BeFalse())
		Expect(len(requestURLs)).To(Equal(2))
	})

	It("Returns false when all nodes are not reachable", func() {
		requestURLs := []string{}
		fakeUrlGetter.GetStub = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return nil, errors.New("Timed out")
		}

		checker := NewClusterHealthChecker([]string{"https://1.2.3.4:9201", "https://5.6.7.8:9201"}, testLogger, fakeUrlGetter)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeFalse())

		Expect(testLogger.LogMessages()).To(ContainElement("cluster_health_checker.checking cluster member health failed"))

		Expect(len(requestURLs)).To(Equal(2))
	})
})
