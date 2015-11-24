package bootstrap_api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/bootstrap_api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Bootstrap API", func() {
	var (
		monitClient *fakes.FakeMonitClient
		ts          *httptest.Server
	)

	BeforeEach(func() {
		monitClient = &fakes.FakeMonitClient{}
		testLogger := lagertest.NewTestLogger("mysql_cmd")
		monitClient.GetLoggerReturns(testLogger)

		testConfig := &config.Config{
			BootstrapEndpoint: config.BootstrapEndpointConfig{
				Username: "fake-username",
				Password: "fake-password",
			},
			Logger: testLogger,
		}

		monitClient.StopServiceReturns(true, nil)
		monitClient.StartServiceReturns(true, nil)
		monitClient.GetStatusReturns("running", nil)

		handler := bootstrap_api.NewHandler(testConfig, monitClient)
		ts = httptest.NewServer(handler)
	})

	AfterEach(func() {
		ts.Close()
	})

	var getEndpoint = func(endpoint string) string {
		return fmt.Sprintf("%s/%s", ts.URL, endpoint)
	}

	It("Calls StopService on the monit client when a stop command is sent", func() {
		resp, err := http.Get(getEndpoint("/stop_mysql"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(monitClient.StopServiceCallCount()).To(Equal(1))
	})

	It("Calls StartService(join) on the monit client when a start command is sent in join mode", func() {
		resp, err := http.Get(getEndpoint("/start_mysql_join"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(monitClient.StartServiceCallCount()).To(Equal(1))
		Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("join"))
	})

	It("Calls StartService(bootstrap) on the monit client when a start command is sent in bootstrap mode", func() {
		resp, err := http.Get(getEndpoint("/start_mysql_bootstrap"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(monitClient.StartServiceCallCount()).To(Equal(1))
		Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("bootstrap"))
	})

	It("Calls GetStatus on the monit client when a new GetStatusCmd is created", func() {
		resp, err := http.Get(getEndpoint("/mysql_status"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(monitClient.GetStatusCallCount()).To(Equal(1))
	})
})
