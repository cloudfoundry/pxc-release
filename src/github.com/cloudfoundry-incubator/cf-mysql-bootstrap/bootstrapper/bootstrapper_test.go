package bootstrapper_test

import (
	"fmt"
	"time"

	"net/http"
	"net/http/httptest"

	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	clockPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Bootstrap", func() {

	const SERVER_COUNT = 3

	var (
		testServers      []*httptest.Server
		endpointHandlers []*test_helpers.EndpointHandler
		rootConfig       *config.Config
		bootstrapper     *bootstrapperPkg.Bootstrapper
	)

	BeforeEach(func() {
		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < SERVER_COUNT; i++ {
			endpointHandler := test_helpers.NewEndpointHandler()
			endpointHandler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			endpointHandlers = append(endpointHandlers, endpointHandler)
		}
	})

	JustBeforeEach(func() {
		rootConfig = &config.Config{
			ShutDownMysql:             "stop_mysql",
			MysqlStatus:               "mysql_status",
			GetSeqNumber:              "sequence_number",
			StartMysqlInJoinMode:      "start_mysql_join",
			StartMysqlInBootstrapMode: "start_mysql_bootstrap",
		}

		rootConfig.Logger = lagertest.NewTestLogger("bootstrapper test")
		testServers = []*httptest.Server{}
		for i := 0; i < SERVER_COUNT; i++ {
			newServer := httptest.NewServer(endpointHandlers[i])
			testServers = append(testServers, newServer)

			rootConfig.HealthcheckURLs = append(rootConfig.HealthcheckURLs, newServer.URL)
		}

		fakeClock := &clockPkg.FakeClock{}
		fakeClock.AfterStub = func(interval time.Duration) <-chan time.Time {
			return time.After(1 * time.Millisecond)
		}
		bootstrapper = bootstrapperPkg.New(rootConfig, fakeClock)
	})

	AfterEach(func() {
		for _, ts := range testServers {
			ts.Close()
		}
	})

	Context("when we get 200 response for shutting down mariadb_ctrl from mysql node", func() {

		const callCountUntilRunning = 5

		BeforeEach(func() {
			for i := 0; i < SERVER_COUNT; i++ {
				currCallCount := 0
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					if currCallCount >= callCountUntilRunning {
						responseText = "stopped"
					} else {
						responseText = "pending"
						currCallCount++
					}
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[i].StubEndpoint("/mysql_status", fakeHandler)
			}
		})

		It("sends request to /stop_mysql on each node and waits for shutdown", func() {
			err := bootstrapper.Run()
			Expect(err).ToNot(HaveOccurred())
			for _, handler := range endpointHandlers {
				Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
			}

			for _, handler := range endpointHandlers {
				Expect(handler.GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", callCountUntilRunning))
			}
		})
	})

	Context("when we get non-200 response for shutting down mariadb_ctrl from mysql node", func() {
		BeforeEach(func() {
			endpointHandlers[0].StubEndpointWithStatus("/stop_mysql",
				http.StatusInternalServerError,
				"fake-error")
		})

		It("returns the error", func() {
			err := bootstrapper.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-error"))

			Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
		})
	})
})
