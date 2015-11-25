package api_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	healthcheckfakes "github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck/fakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client/fakes"
	sequencefakes "github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

const (
	ExpectedSeqno             = "4"
	ExpectedHealthCheckStatus = "synced"
)

var _ = Describe("Bootstrap API", func() {
	var (
		monitClient    *fakes.FakeMonitClient
		sequenceNumber *sequencefakes.FakeSequenceNumberChecker
		healthchecker  *healthcheckfakes.FakeHealthChecker
		ts             *httptest.Server
	)

	BeforeEach(func() {
		monitClient = &fakes.FakeMonitClient{}
		sequenceNumber = &sequencefakes.FakeSequenceNumberChecker{}
		sequenceNumber.CheckReturns(ExpectedSeqno, nil)

		healthchecker = &healthcheckfakes.FakeHealthChecker{}
		healthchecker.CheckReturns(ExpectedHealthCheckStatus, nil)

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

		handler := api.NewHandler(api.ApiParameters{
			RootConfig:            testConfig,
			MonitClient:           monitClient,
			SequenceNumberChecker: sequenceNumber,
			Healthchecker:         healthchecker,
		})
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

	It("Calls Checker on the SequenceNumberchecker when a new sequence_number is created", func() {
		resp, err := http.Get(getEndpoint("/sequence_number"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		responseBody, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(responseBody).To(ContainSubstring(ExpectedSeqno))
		Expect(sequenceNumber.CheckCallCount()).To(Equal(1))
	})

	It("Calls Check on the Healthchecker at the root endpoint", func() {
		resp, err := http.Get(getEndpoint("/"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		responseBody, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
		Expect(healthchecker.CheckCallCount()).To(Equal(1))
	})

	It("Calls Check on the Healthchecker at /galera_status", func() {
		resp, err := http.Get(getEndpoint("/galera_status"))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		responseBody, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
		Expect(healthchecker.CheckCallCount()).To(Equal(1))
	})

})
