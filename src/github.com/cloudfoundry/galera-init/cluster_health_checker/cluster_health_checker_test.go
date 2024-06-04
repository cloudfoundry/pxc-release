package cluster_health_checker_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	. "github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
	"github.com/cloudfoundry/galera-init/test_helpers"
)

var _ = Describe("ClusterHealthChecker.HealthyCluster()", func() {
	var testLogger *slog.Logger
	var slogHandler *test_helpers.SlogHandler
	var fakeUrlGetter *cluster_health_checkerfakes.FakeUrlGetter

	BeforeEach(func() {
		fakeUrlGetter = &cluster_health_checkerfakes.FakeUrlGetter{}
		slogHandler = test_helpers.NewSlogHandler()
		testLogger = slog.New(slogHandler)
	})

	It("Immediately returns true when a reachable node returns healthy", func() {
		var requestURLs []string
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
		var requestURLs []string
		fakeUrlGetter.GetStub = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: io.NopCloser(bytes.NewBufferString("some body"))}, nil
		}

		checker := NewClusterHealthChecker([]string{"https://1.2.3.4:9201", "https://5.6.7.8:9201"}, testLogger, fakeUrlGetter)
		healthy := checker.HealthyCluster()

		Expect(slogHandler.Messages).To(ContainElement("node https://1.2.3.4:9201 is NOT healthy"))
		Expect(slogHandler.Messages).To(ContainElement("node https://5.6.7.8:9201 is NOT healthy"))
		Expect(slogHandler.Buffer).To(gbytes.Say(`"status":"503 Service Unavailable"`))
		Expect(slogHandler.Buffer).To(gbytes.Say(`"body":"some body"`))

		Expect(healthy).To(BeFalse())
		Expect(len(requestURLs)).To(Equal(2))
	})

	It("Returns false when all nodes are not reachable", func() {
		var requestURLs []string
		fakeUrlGetter.GetStub = func(url string) (*http.Response, error) {
			requestURLs = append(requestURLs, url)
			return nil, errors.New("Timed out")
		}

		checker := NewClusterHealthChecker([]string{"https://1.2.3.4:9201", "https://5.6.7.8:9201"}, testLogger, fakeUrlGetter)
		healthy := checker.HealthyCluster()

		Expect(healthy).To(BeFalse())

		Expect(slogHandler.Messages).To(ContainElement("checking cluster member health failed"))

		Expect(len(requestURLs)).To(Equal(2))
	})
})
