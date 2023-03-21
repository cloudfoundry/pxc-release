package node_manager_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"
	clockPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/test_helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	ServerCount     = 3
	ArbitratorIndex = 1
)

var (
	testServers      []*httptest.Server
	endpointHandlers []*test_helpers.EndpointHandler
	rootConfig       *config.Config
	nodeManager      node_manager.NodeManager
)

var _ = Describe("Bootstrap", func() {
	node_manager.GetShutDownTimeout = func() int {
		return 10
	}

	BeforeEach(func() {

		rootConfig = &config.Config{
			ShutDownMysql:             "stop_mysql",
			MysqlStatus:               "mysql_status",
			GetSeqNumber:              "sequence_number",
			StartMysqlInJoinMode:      "start_mysql_join",
			StartMysqlInBootstrapMode: "start_mysql_bootstrap",
		}

		rootConfig.Logger = lagertest.NewTestLogger("nodeManager test")

		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < ServerCount; i++ {
			endpointHandlers = append(endpointHandlers, test_helpers.NewEndpointHandler())
		}
	})

	JustBeforeEach(func() {
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
		Context("when all nodes are full mysql nodes", func() {
			Context("when all mysql nodes are unhealthy", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
					}
				})

				Context("when RepairMode is bootstrap", func() {
					It("does not return an error", func() {
						rootConfig.RepairMode = "bootstrap"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).ToNot(HaveOccurred())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("returns an error", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal("More than one node is unhealthy, cannot execute rejoin-unsafe."))
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})
			})

			Context("when all mysql nodes are healthy", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
				})

				Context("when RepairMode is bootstrap", func() {
					It("returns false,nil", func() {
						rootConfig.RepairMode = "bootstrap"
						clusterUnhealthy, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterUnhealthy).To(BeFalse())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("returns false,nil", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						clusterUnhealthy, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterUnhealthy).To(BeFalse())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})
			})

			Context("when all but one mysql nodes are synced", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
					endpointHandlers[ServerCount-1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
				})

				Context("when RepairMode is bootstrap", func() {
					It("returns an error without bootstrapping", func() {
						rootConfig.RepairMode = "bootstrap"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("one or more nodes are failing"))
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("does not return an error", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})
			})
		})

		Context("when there is an arbitrator node", func() {
			Context("when all mysql nodes are unhealthy", func() {
				BeforeEach(func() {
					for i, handler := range endpointHandlers {
						if i == ArbitratorIndex {
							handler.StubEndpointWithStatus("/", http.StatusBadRequest, "arbitrator node")
						} else {
							handler.StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
						}
					}
				})

				Context("when RepairMode is bootstrap", func() {
					It("does not return an error", func() {
						rootConfig.RepairMode = "bootstrap"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).ToNot(HaveOccurred())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("returns an error", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal("More than one node is unhealthy, cannot execute rejoin-unsafe."))
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})
			})

			Context("when all mysql nodes are healthy", func() {
				BeforeEach(func() {
					for i, handler := range endpointHandlers {
						if i == ArbitratorIndex {
							handler.StubEndpointWithStatus("/", http.StatusBadRequest, "arbitrator node")
						} else {
							handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
						}
					}
				})

				Context("when RepairMode is bootstrap", func() {
					It("returns false,nil", func() {
						rootConfig.RepairMode = "bootstrap"
						clusterUnhealthy, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterUnhealthy).To(BeFalse())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("returns false,nil", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						clusterUnhealthy, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterUnhealthy).To(BeFalse())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})
			})

			Context("when all but one mysql nodes are synced", func() {
				BeforeEach(func() {
					for i, handler := range endpointHandlers {
						if i == ArbitratorIndex {
							handler.StubEndpointWithStatus("/", http.StatusBadRequest, "arbitrator node")
						} else {
							handler.StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
						}
					}
					endpointHandlers[ServerCount-1].StubEndpointWithStatus("/", http.StatusOK, "synced")
				})

				Context("when RepairMode is bootstrap", func() {
					It("returns an error without bootstrapping", func() {
						rootConfig.RepairMode = "bootstrap"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("one or more nodes are failing"))
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

				Context("when RepairMode is rejoin-unsafe", func() {
					It("does not return an error", func() {
						rootConfig.RepairMode = "rejoin-unsafe"
						_, err := nodeManager.VerifyClusterIsUnhealthy()
						Expect(err).NotTo(HaveOccurred())
						for _, handler := range endpointHandlers {
							Expect(handler.GetFakeHandler("/").ServeHTTPCallCount()).To(Equal(1))
						}
					})
				})

			})
		})
	})

	Describe("#FindUnhealthyNode", func() {
		Context("when one node is unhealthy", func() {
			Context("when all are mysql nodes", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
					endpointHandlers[1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
				})

				It("returns the url of the unhealthy node", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).ToNot(HaveOccurred())
					Expect(unhealthyNodeURL).To(Equal(testServers[1].URL))
				})
			})

			Context("when we have one arbitrator", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
					endpointHandlers[0].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "arbitrator")
					endpointHandlers[1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
				})

				It("returns the url of the unhealthy node", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).ToNot(HaveOccurred())
					Expect(unhealthyNodeURL).To(Equal(testServers[1].URL))
				})
			})
		})

		Context("when no nodes are unhealthy", func() {
			Context("when all are mysql nodes", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
				})

				It("returns an error", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("Found no unhealthy nodes"))
					Expect(unhealthyNodeURL).To(Equal(""))
				})
			})

			Context("when we have one arbitrator", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
					endpointHandlers[1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "arbitrator")
				})

				It("returns an error", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("Found no unhealthy nodes"))
					Expect(unhealthyNodeURL).To(Equal(""))
				})
			})
		})

		Context("when more than one nodes are unhealthy", func() {
			Context("when all are mysql nodes", func() {
				BeforeEach(func() {
					for _, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/", http.StatusOK, "synced")
					}
					endpointHandlers[1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
					endpointHandlers[2].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
				})

				It("returns an error", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("Found more than one unhealthy node"))
					Expect(unhealthyNodeURL).To(Equal(""))
				})
			})

			Context("when we have one arbitrator", func() {
				BeforeEach(func() {
					endpointHandlers[0].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "arbitrator")
					endpointHandlers[1].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
					endpointHandlers[2].StubEndpointWithStatus("/", http.StatusServiceUnavailable, "not synced")
				})

				It("returns an error", func() {
					unhealthyNodeURL, err := nodeManager.FindUnhealthyNode()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("Found more than one unhealthy node"))
					Expect(unhealthyNodeURL).To(Equal(""))
				})
			})
		})
	})

	Describe("#StopNode", func() {
		Context("when the shutdown request succeeds", func() {
			const pendingCallCount = 3
			BeforeEach(func() {
				node_manager.GetShutDownTimeout = func() int {
					return 25
				}
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
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			})

			It("stops the node and quits successfully", func() {
				err := nodeManager.StopNode(testServers[0].URL)
				Expect(err).ToNot(HaveOccurred())
				Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})

		})

		Context("when the shutdown request fails", func() {
			const pendingCallCount = 3
			BeforeEach(func() {
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					responseText = "pending"
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/stop_mysql", http.StatusInternalServerError)
			})

			It("fails to stop the node and returns error", func() {
				err := nodeManager.StopNode(testServers[0].URL)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to stop"))
				Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(Equal(0))
			})
		})

		Context("when the node does not shut down in time", func() {
			const pendingCallCount = 3
			BeforeEach(func() {
				node_manager.GetShutDownTimeout = func() int {
					return 25
				}
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					responseText = "pending"
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			})

			It("fails to stop the node and returns error", func() {
				err := nodeManager.StopNode(testServers[0].URL)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to stop"))
				Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

	})

	Describe("#VerifyAllNodesAreReachable", func() {

		Context("when all nodes are reachable", func() {

			BeforeEach(func() {
				for _, handler := range endpointHandlers {
					handler.StubEndpointWithStatus("/mysql_status", http.StatusOK)
				}
			})

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

			const pendingCallCount = 3

			BeforeEach(func() {
				node_manager.GetShutDownTimeout = func() int {
					return 25
				}
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
				for _, handler := range endpointHandlers {
					handler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
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
				for _, handler := range endpointHandlers {
					handler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
					handler.StubEndpointWithStatus("/mysql_status", http.StatusOK, "stopped")
				}
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

				expectedMaxIterations := node_manager.GetShutDownTimeout() / node_manager.PollingIntervalInSec
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", expectedMaxIterations))
			})
		})
	})

	Describe("#GetSequenceNumbers", func() {
		Context("when all nodes are full mysql nodes", func() {
			Context("when all nodes return a valid seqno", func() {
				BeforeEach(func() {
					for i, handler := range endpointHandlers {
						handler.StubEndpointWithStatus("/sequence_number", http.StatusOK, strconv.Itoa(i))
					}
				})

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
					endpointHandlers[0].StubEndpointWithStatus("/sequence_number", http.StatusServiceUnavailable, "fake-error")
				})

				It("returns an error", func() {
					_, err := nodeManager.GetSequenceNumbers()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("fake-error"))

					Expect(endpointHandlers[0].GetFakeHandler("/sequence_number").ServeHTTPCallCount()).To(Equal(1))
				})
			})
		})

		Context("when there is an arbitrator node", func() {
			Context("when all mysql nodes return a valid seqno", func() {
				BeforeEach(func() {
					for i, handler := range endpointHandlers {
						if i == ArbitratorIndex {
							handler.StubEndpointWithStatus("/sequence_number", http.StatusOK, "no sequence number - running on arbitrator node")
						} else {
							handler.StubEndpointWithStatus("/sequence_number", http.StatusOK, strconv.Itoa(i))
						}
					}
				})

				It("it sets the arbitrator's sequence number to -1", func() {
					urlToSeqno, err := nodeManager.GetSequenceNumbers()
					Expect(err).ToNot(HaveOccurred())

					for _, handler := range endpointHandlers {
						Expect(handler.GetFakeHandler("/sequence_number").ServeHTTPCallCount()).To(Equal(1))
					}

					Expect(urlToSeqno).To(HaveLen(len(rootConfig.HealthcheckURLs)))
					for i, url := range rootConfig.HealthcheckURLs {
						Expect(urlToSeqno).To(HaveKey(url))
						if i == ArbitratorIndex {
							Expect(urlToSeqno[url]).To(Equal(-1))
						} else {
							Expect(urlToSeqno[url]).To(Equal(i))
						}
					}
				})
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

		Context("when the bootstrap request fails", func() {

			const pendingCallCount = 5

			BeforeEach(func() {
				currCallCount := 0
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					if currCallCount <= pendingCallCount {
						responseText = "pending"
					} else {
						responseText = "failing"
					}
					currCallCount++
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_bootstrap", http.StatusOK)
			})

			It("sends a bootstrap command to a node and returns an error when it reports 'failing'", func() {
				bootstrapNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.BootstrapNode(bootstrapNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("Node is failing: %s", bootstrapNodeUrl)))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

		Context("when the bootstrap endpoint returns a non-200 response", func() {

			BeforeEach(func() {
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_bootstrap", http.StatusServiceUnavailable, "fake-error")
			})

			It("returns an error", func() {
				bootStrapNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.BootstrapNode(bootStrapNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_bootstrap").ServeHTTPCallCount()).To(Equal(1))
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

				_, joinReq := endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPArgsForCall(0)
				Expect(joinReq.URL.Query().Get("sst")).To(Equal("true"))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

		Context("when the join request fails", func() {

			const pendingCallCount = 5

			BeforeEach(func() {
				currCallCount := 0
				fakeHandler := &fakes.FakeHandler{}
				fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
					var responseText string
					if currCallCount <= pendingCallCount {
						responseText = "pending"
					} else {
						responseText = "failing"
					}
					currCallCount++
					fmt.Fprintf(w, responseText)
				}
				endpointHandlers[0].StubEndpoint("/mysql_status", fakeHandler)
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_join", http.StatusOK)
			})

			It("sends a join command to a node and returns an error when it reports 'failing'", func() {
				joinNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.JoinNode(joinNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("Node is failing: %s", joinNodeUrl)))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))
				Expect(endpointHandlers[0].GetFakeHandler("/mysql_status").ServeHTTPCallCount()).To(BeNumerically(">=", pendingCallCount))
			})
		})

		Context("when the join endpoint returns a non-200 response", func() {

			BeforeEach(func() {
				endpointHandlers[0].StubEndpointWithStatus("/start_mysql_join", http.StatusInternalServerError, "fake-error")
			})

			It("returns an error", func() {
				joinNodeUrl := rootConfig.HealthcheckURLs[0]

				err := nodeManager.JoinNode(joinNodeUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))

				Expect(endpointHandlers[0].GetFakeHandler("/start_mysql_join").ServeHTTPCallCount()).To(Equal(1))
			})
		})
	})
})

var _ = Describe("TLS connectivity", func() {
	BeforeEach(func() {
		rootConfig = &config.Config{
			ShutDownMysql:             "stop_mysql",
			MysqlStatus:               "mysql_status",
			GetSeqNumber:              "sequence_number",
			StartMysqlInJoinMode:      "start_mysql_join",
			StartMysqlInBootstrapMode: "start_mysql_bootstrap",
		}
		rootConfig.BackendTLS = config.BackendTLS{
			Enabled:            true,
			ServerName:         "example.com", // set in TLS server by httptest
			CA:                 "unavailableInTest",
			InsecureSkipVerify: true, // req'd for TLS to mock httptest nodes
		}
		rootConfig.Logger = lagertest.NewTestLogger("nodeManager test")

		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < ServerCount; i++ {
			newHandler := test_helpers.NewEndpointHandler()
			newHandler.StubEndpointWithStatus("/mysql_status", http.StatusOK)
			endpointHandlers = append(endpointHandlers, newHandler)
		}
	})
	JustBeforeEach(func() {
		testServers = []*httptest.Server{}
		for i := 0; i < ServerCount; i++ {
			newServer := httptest.NewTLSServer(endpointHandlers[i])
			testServers = append(testServers, newServer)
			rootConfig.HealthcheckURLs = append(rootConfig.HealthcheckURLs, newServer.URL)
		}

		fakeClock := &clockPkg.FakeClock{}
		fakeClock.AfterStub = func(interval time.Duration) <-chan time.Time {
			return time.After(1 * time.Millisecond)
		}
		nodeManager = node_manager.New(rootConfig, fakeClock)
	})
	When("TLS is properly configured", func() {
		It("connects via TLS", func() {
			err := nodeManager.VerifyAllNodesAreReachable()
			Expect(err).ToNot(HaveOccurred())
		})
	})
	When("the client attempts a non-TLS connection to a TLS back-end", func() {
		BeforeEach(func() {
			rootConfig.BackendTLS.Enabled = false
		})
		It("returns the expected error", func() {
			err := nodeManager.VerifyAllNodesAreReachable()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("Could not reach node: https://"))
			Expect(err.Error()).To(SatisfyAny(
				HaveSuffix("x509: certificate signed by unknown authority"),
				MatchRegexp(`x509:.*certificate is not trusted$`),
			))
		})
	})
	When("the server's certificate cannot be authenticated", func() {
		BeforeEach(func() {
			// Since httptest nodes don't support secure TLS connections,
			// this provokes the desired authentication failure.
			rootConfig.BackendTLS.InsecureSkipVerify = false
		})
		It("returns the expected error", func() {
			err := nodeManager.VerifyAllNodesAreReachable()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HaveSuffix("x509: certificate signed by unknown authority"))
		})
	})
	When("the server's certificate doesn't contains the expected name", func() {
		BeforeEach(func() {
			rootConfig.BackendTLS.InsecureSkipVerify = false
			rootConfig.BackendTLS.ServerName = "incorrectValue.org"
		})
		It("returns the expected error", func() {
			err := nodeManager.VerifyAllNodesAreReachable()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HaveSuffix("x509: certificate is valid for example.com, not incorrectValue.org"))
		})
	})

})
