package bootstrapper_test

import (
	"fmt"
	"strconv"
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

const SERVER_COUNT = 3

var (
	testServers      []*httptest.Server
	endpointHandlers []*test_helpers.EndpointHandler
	rootConfig       *config.Config
	bootstrapper     *bootstrapperPkg.Bootstrapper
)

var _ = Describe("Bootstrap", func() {

	BeforeEach(func() {
		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < SERVER_COUNT; i++ {
			endpointHandler := test_helpers.NewEndpointHandler()
			endpointHandler.StubEndpointWithStatus("/", http.StatusOK)
			endpointHandler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			endpointHandler.StubEndpointWithStatus("/sequence_number", http.StatusOK, strconv.Itoa(i))
			if i == SERVER_COUNT-1 {
				endpointHandler.StubEndpointWithStatus("/start_mysql_bootstrap", http.StatusOK)
			} else {
				endpointHandler.StubEndpointWithStatus("/start_mysql_join", http.StatusOK)
			}
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

	Context("when all mysql nodes are already synced", func() {

		BeforeEach(func() {
			for i := 0; i < SERVER_COUNT; i++ {
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					responseText = "synced"
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[i].StubEndpoint("/", fakeHandler)
			}
		})

		It("errors without bootstrapping", func() {
			err := bootstrapper.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("All nodes are synced. Bootstrap not required."))
			for _, handler := range endpointHandlers {
				Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(0))
			}
		})
	})

	Context("when some mysql nodes are synced but some are unhealthy", func() {

		BeforeEach(func() {
			for i := 0; i < SERVER_COUNT; i++ {
				if i < SERVER_COUNT-1 {
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						var responseText string
						responseText = "synced"
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/", fakeHandler)
				} else {
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusServiceUnavailable)
						var responseText string
						responseText = "not synced"
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/", fakeHandler)
				}
			}
		})

		It("returns an error without bootstrapping", func() {
			err := bootstrapper.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("one or more nodes are failing"))
			for _, handler := range endpointHandlers {
				Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(0))
			}
		})

	})

	Context("when mysql nodes need bootstrap and ", func() {

		BeforeEach(func() {
			makeFailingCluster()
		})

		Context("when we get 200 response for shutting down mariadb_ctrl from a mysql node", func() {

			const stoppedCallCount = 5
			const runningCallCount = 10

			BeforeEach(func() {
				for i := 0; i < SERVER_COUNT; i++ {
					currCallCount := 0
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						var responseText string
						if currCallCount == stoppedCallCount {
							responseText = "stopped"
						} else if currCallCount == runningCallCount {
							responseText = "running"
						} else {
							responseText = "pending"
						}
						currCallCount++
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/mysql_status", fakeHandler)
				}
			})

			It("bootstraps", func() {
				err := bootstrapper.Run()
				Expect(err).ToNot(HaveOccurred())
				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
				}

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", stoppedCallCount))
				}

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/sequence_number").ServeHTTPCallCount()).To(Equal(1))
				}

				Expect(endpointHandlers[SERVER_COUNT-1].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[SERVER_COUNT-1].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", runningCallCount))

				for i, handler := range endpointHandlers {
					if i < (SERVER_COUNT - 1) {
						Expect(handler.GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))
						Expect(handler.GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", runningCallCount))
					}
				}
			})
		})

	})

	Context("when we get non-200 response for shutting down mariadb_ctrl from mysql node", func() {
		BeforeEach(func() {
			makeFailingCluster()
			endpointHandlers[0].StubEndpointWithStatus("/stop_mysql",
				http.StatusInternalServerError,
				"fake-error")
		})

		It("returns error and quits", func() {
			err := bootstrapper.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-error"))

			Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
		})
	})
})

func makeFailingCluster() {
	for i := 0; i < SERVER_COUNT; i++ {
		fakeHandler := &fakes.FakeHandler{}
		fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			var responseText string
			responseText = "not synced"
			fmt.Fprintf(w, responseText)
		}
		endpointHandlers[i].StubEndpoint("/", fakeHandler)
	}
}
