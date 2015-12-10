package node_manager_test

import (
	"fmt"
	"strconv"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"
	clockPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

const (
	ServerCount    = 3
	StartupTimeout = 600
)

var (
	testServers      []*httptest.Server
	endpointHandlers []*test_helpers.EndpointHandler
	rootConfig       *config.Config
	nodeManager      node_manager.NodeManager
)

var _ = Describe("Bootstrap", func() {

	BeforeEach(func() {
		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < ServerCount; i++ {
			endpointHandler := test_helpers.NewEndpointHandler()
			endpointHandler.StubEndpointWithStatus("/", http.StatusOK)
			endpointHandler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			endpointHandler.StubEndpointWithStatus("/sequence_number", http.StatusOK, strconv.Itoa(i))
			endpointHandler.StubEndpointWithStatus("/mysql_status", http.StatusOK, "running")
			if i == ServerCount-1 {
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
			DatabaseStartupTimeout:    StartupTimeout,
		}

		rootConfig.Logger = lagertest.NewTestLogger("nodeManager test")
		testServers = []*httptest.Server{}
		for i := 0; i < ServerCount; i++ {
			newServer := httptest.NewServer(endpointHandlers[i])
			testServers = append(testServers, newServer)

			rootConfig.HealthcheckURLs = append(rootConfig.HealthcheckURLs, newServer.URL)
		}

		fakeClock := &clockPkg.FakeClock{}
		fakeClock.AfterStub = func(interval time.Duration) <-chan time.Time {
			return time.After(1 * time.Millisecond)
		}
		nodeManager = node_manager.New(rootConfig, fakeClock)
	})

	AfterEach(func() {
		for _, ts := range testServers {
			ts.Close()
		}
	})

	Describe("#VerifyClusterIsUnhealthy", func() {

		Context("when all mysql nodes are unhealthy", func() {

			BeforeEach(func() {
				for i := 0; i < ServerCount; i++ {
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusServiceUnavailable)
						var responseText string
						responseText = "not synced"
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/", fakeHandler)
				}
			})

			It("does not return an error", func() {
				err := nodeManager.VerifyClusterIsUnhealthy()
				Expect(err).ToNot(HaveOccurred())
				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
				}
			})
		})

		Context("when all mysql nodes are healthy", func() {

			BeforeEach(func() {
				for i := 0; i < ServerCount; i++ {
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						var responseText string
						responseText = "synced"
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/", fakeHandler)
				}
			})

			It("returns an error", func() {
				err := nodeManager.VerifyClusterIsUnhealthy()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("All nodes are synced. Bootstrap not required."))
				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
				}
			})
		})

		Context("when some mysql nodes are synced but some are unhealthy", func() {

			BeforeEach(func() {
				for i := 0; i < ServerCount; i++ {
					if i < ServerCount-1 {
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
				err := nodeManager.VerifyClusterIsUnhealthy()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("one or more nodes are failing"))
				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
				}
			})

		})
	})

	Describe("#VerifyAllNodesAreReachable", func() {

		Context("when all nodes are reachable", func() {
			It("does not return an error", func() {
				err := nodeManager.VerifyAllNodesAreReachable()
				Expect(err).ToNot(HaveOccurred())

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(Equal(1))
				}
			})
		})

		Context("when any node returns a non-200 response", func() {
			BeforeEach(func() {
				makeFailingCluster()
				endpointHandlers[0].StubEndpointWithStatus("/mysql_status",
					http.StatusInternalServerError,
					"fake-error")
			})

			It("returns an error", func() {
				err := nodeManager.VerifyAllNodesAreReachable()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(Equal(1))
			})
		})

		Context("when we cannot get a response from healthcheck", func() {
			JustBeforeEach(func() {
				for _, ts := range testServers {
					ts.Close()
				}
			})

			It("returns an error", func() {
				err := nodeManager.VerifyAllNodesAreReachable()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Could not reach node"))
			})
		})
	})

	Describe("#StopAllNodes", func() {
		Context("when all shutdown requests succeed", func() {

			const pendingCallCount = 5

			BeforeEach(func() {
				for i := 0; i < ServerCount; i++ {
					currCallCount := 0
					fakeHandler := &fakes.FakeHandler{}
					fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
						var responseText string
						if currCallCount <= pendingCallCount {
							responseText = "pending"
						} else {
							responseText = "stopped"
						}
						currCallCount++
						fmt.Fprintf(w, responseText)
					}
					endpointHandlers[i].StubEndpoint("/mysql_status", fakeHandler)
				}
			})

			It("returns the error and quits", func() {
				err := nodeManager.StopAllNodes()
				Expect(err).ToNot(HaveOccurred())

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
					Expect(handler.GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
				}
			})
		})

		Context("when any shutdown request to mariadb_ctrl fails", func() {
			BeforeEach(func() {
				makeFailingCluster()
				endpointHandlers[0].StubEndpointWithStatus("/stop_mysql",
					http.StatusInternalServerError,
					"fake-error")
			})

			It("returns the error and quits", func() {
				err := nodeManager.StopAllNodes()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
			})
		})

		Context("when we timeout waiting for mariadb_ctrl to shutdown", func() {
			BeforeEach(func() {
				makeFailingCluster()
				endpointHandlers[0].StubEndpointWithStatus("/mysql_status",
					http.StatusOK,
					"pending")
			})

			It("returns timeout error and quits", func() {
				err := nodeManager.StopAllNodes()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Timed out"))

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
				}

				expectedMaxIterations := StartupTimeout / node_manager.PollingIntervalInSec
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", expectedMaxIterations))
			})
		})
	})

	Describe("#GetSequenceNumbers", func() {
		Context("when all nodes return a valid seqno", func() {
			It("returns a map from URL to seqno", func() {
				urlToSeqno, err := nodeManager.GetSequenceNumbers()
				Expect(err).ToNot(HaveOccurred())

				for _, handler := range endpointHandlers {
					Expect(handler.GetFakeHandler("/sequence_number").ServeHTTPCallCount()).To(Equal(1))
				}

				Expect(urlToSeqno).To(HaveLen(len(rootConfig.HealthcheckURLs)))
				for i, url := range rootConfig.HealthcheckURLs {
					Expect(urlToSeqno).To(HaveKey(url))
					Expect(urlToSeqno[url]).To(Equal(i))
				}
			})
		})

		Context("when any node returns a non-valid sequence number", func() {
			BeforeEach(func() {
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
					responseText := "fake-error"
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/sequence_number", fakeHandler)
			})
			It("returns an error", func() {
				_, err := nodeManager.GetSequenceNumbers()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/sequence_number").ServeHTTPCallCount()).To(Equal(1))
			})
		})
	})

	Describe("#BootstrapNode", func() {

		Context("when the bootstrap request succeeds", func() {

			const pendingCallCount = 5

			BeforeEach(func() {
				currCallCount := 0
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					if currCallCount <= pendingCallCount {
						responseText = "pending"
					} else {
						responseText = "running"
					}
					currCallCount++
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_bootstrap", http.StatusOK)
			})

			It("sends a bootstrap command to a node and waits for it to be 'running'", func() {
				bootStrapNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.BootstrapNode(bootStrapNodeUrl)
				Expect(err).ToNot(HaveOccurred())

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

		Context("when the bootstrap endpoint returns a non-200 response", func() {

			BeforeEach(func() {
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, "fake-error")
				}
				endpointHandlers[0].StubEndpoint("/start_mysql_bootstrap", fakeHandler)
			})

			It("returns an error", func() {
				bootStrapNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.BootstrapNode(bootStrapNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))
			})
		})

		Context("when we timeout waiting for the node to bootstrap", func() {
			BeforeEach(func() {
				endpointHandlers[0].StubEndpointWithStatus("/mysql_status",
					http.StatusOK,
					"pending")
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_bootstrap", http.StatusOK)
			})

			It("returns timeout error and quits", func() {
				bootStrapNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.BootstrapNode(bootStrapNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Timed out"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))

				expectedMaxIterations := StartupTimeout / node_manager.PollingIntervalInSec
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", expectedMaxIterations))
			})
		})
	})

	Describe("#JoinNode", func() {

		Context("when the join request succeeds", func() {

			const pendingCallCount = 5

			BeforeEach(func() {
				currCallCount := 0
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					if currCallCount <= pendingCallCount {
						responseText = "pending"
					} else {
						responseText = "running"
					}
					currCallCount++
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_join", http.StatusOK)
			})

			It("sends a join command to a node and waits for it to be 'running'", func() {
				joinNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.JoinNode(joinNodeUrl)
				Expect(err).ToNot(HaveOccurred())

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

		Context("when the join endpoint returns a non-200 response", func() {

			BeforeEach(func() {
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, "fake-error")
				}
				endpointHandlers[0].StubEndpoint("/start_mysql_join", fakeHandler)
			})

			It("returns an error", func() {
				joinNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.JoinNode(joinNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))
			})
		})

		Context("when we timeout waiting for the node to join", func() {
			BeforeEach(func() {
				endpointHandlers[0].StubEndpointWithStatus("/mysql_status",
					http.StatusOK,
					"pending")
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_join", http.StatusOK)
			})

			It("returns timeout error and quits", func() {
				joinNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.JoinNode(joinNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Timed out"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))

				expectedMaxIterations := StartupTimeout / node_manager.PollingIntervalInSec
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", expectedMaxIterations))
			})
		})
	})
})

func makeFailingCluster() {
	for i := 0; i < ServerCount; i++ {
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
