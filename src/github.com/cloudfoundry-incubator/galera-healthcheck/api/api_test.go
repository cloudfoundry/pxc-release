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
	ApiUsername               = "fake-username"
	ApiPassword               = "fake-password"
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
				Username: ApiUsername,
				Password: ApiPassword,
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

	Context("when request has basic auth", func() {

		var getReq = func(endpoint string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest("GET", url, nil)
			Expect(err).ToNot(HaveOccurred())

			req.SetBasicAuth(ApiUsername, ApiPassword)
			return req
		}

		It("Calls StopService on the monit client when a stop command is sent", func() {
			req := getReq("stop_mysql")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(monitClient.StopServiceCallCount()).To(Equal(1))
		})

		It("Calls StartService(join) on the monit client when a start command is sent in join mode", func() {
			resp, err := http.DefaultClient.Do(getReq("start_mysql_join"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceCallCount()).To(Equal(1))
			Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("join"))
		})

		It("Calls StartService(bootstrap) on the monit client when a start command is sent in bootstrap mode", func() {
			resp, err := http.DefaultClient.Do(getReq("start_mysql_bootstrap"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceCallCount()).To(Equal(1))
			Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("bootstrap"))
		})

		It("Calls GetStatus on the monit client when a new GetStatusCmd is created", func() {
			resp, err := http.DefaultClient.Do(getReq("mysql_status"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.GetStatusCallCount()).To(Equal(1))
		})

		It("Calls Checker on the SequenceNumberchecker when a new sequence_number is created", func() {
			resp, err := http.DefaultClient.Do(getReq("sequence_number"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(1))
		})
	})

	Context("when request does not have basic auth", func() {
		var getReq = func(endpoint string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest("GET", url, nil)
			Expect(err).ToNot(HaveOccurred())
			return req
		}

		It("requires authentication for /stop_mysql", func() {
			resp, err := http.DefaultClient.Do(getReq("stop_mysql"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StopServiceCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_bootstrap", func() {
			resp, err := http.DefaultClient.Do(getReq("start_mysql_bootstrap"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_join", func() {
			resp, err := http.DefaultClient.Do(getReq("start_mysql_join"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceCallCount()).To(Equal(0))
		})

		It("requires authentication for /mysql_status", func() {
			resp, err := http.DefaultClient.Do(getReq("mysql_status"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.GetStatusCallCount()).To(Equal(0))
		})

		It("requires authentication for /sequence_number", func() {
			resp, err := http.DefaultClient.Do(getReq("sequence_number"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).ToNot(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(0))
		})

		It("Calls Check on the Healthchecker at the root endpoint", func() {
			resp, err := http.DefaultClient.Do(getReq(""))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})

		It("Calls Check on the Healthchecker at /galera_status", func() {
			resp, err := http.DefaultClient.Do(getReq("galera_status"))
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})
	})
})
