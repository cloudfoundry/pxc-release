package cluster_health_checker_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	. "github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
)

var _ = Describe("ClusterHealthChecker.HealthyCluster()", func() {
	var testLogger *slog.Logger
	var slogHandler *SlogHander
	var fakeUrlGetter *cluster_health_checkerfakes.FakeUrlGetter

	BeforeEach(func() {
		fakeUrlGetter = &cluster_health_checkerfakes.FakeUrlGetter{}
		slogHandler = NewSlogHander()
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

type SlogHander struct {
	handler  slog.Handler
	Buffer   *gbytes.Buffer
	Messages []string
}

func NewSlogHander() *SlogHander {
	buffer := gbytes.NewBuffer()

	return &SlogHander{
		handler: slog.NewJSONHandler(buffer, nil),
		Buffer:  buffer,
	}
}

func (s *SlogHander) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s *SlogHander) Handle(ctx context.Context, record slog.Record) error {
	s.Messages = append(s.Messages, record.Message)
	return s.handler.Handle(ctx, record)
}

func (s *SlogHander) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.handler.WithAttrs(attrs)
}

func (s *SlogHander) WithGroup(name string) slog.Handler {
	return s.handler.WithGroup(name)
}

var _ slog.Handler = &SlogHander{}
