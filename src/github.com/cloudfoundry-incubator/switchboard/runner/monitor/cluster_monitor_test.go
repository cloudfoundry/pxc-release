package monitor_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager/v3/lagertest"

	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor/monitorfakes"
)

const healthcheckTimeout = 500 * time.Millisecond
const eventuallyTimeout = 3 * time.Second

var _ = Describe("ClusterMonitor", func() {
	var (
		backends                     []*domain.Backend
		logger                       *lagertest.TestLogger
		clusterMonitor               *monitor.ClusterMonitor
		backend1, backend2, backend3 *domain.Backend
		subscriberA                  chan *domain.Backend
		subscriberB                  chan *domain.Backend
		useLowestIndex               bool
		urlGetter                    *monitorfakes.FakeUrlGetter
		useTLSForAgent               bool
		m                            sync.RWMutex
	)

	BeforeEach(func() {
		useTLSForAgent = false
		urlGetter = new(monitorfakes.FakeUrlGetter)
		clusterMonitor = nil

		logger = lagertest.NewTestLogger("ClusterMonitor test")

		SetDefaultEventuallyTimeout(eventuallyTimeout)

		backend1 = domain.NewBackend(
			"backend-1",
			"10.10.1.2",
			1337,
			1338,
			"api/v1/status",
			logger,
		)

		m.Lock()
		backend2 = domain.NewBackend(
			"backend-2",
			"10.10.2.2",
			1337,
			1338,
			"api/v1/status",
			logger,
		)
		m.Unlock()

		backend3 = domain.NewBackend(
			"backend-3",
			"10.10.3.2",
			1337,
			1338,
			"api/v1/status",
			logger,
		)

		backends = []*domain.Backend{
			backend1,
			backend2,
			backend3,
		}

		subscriberA = make(chan *domain.Backend, 100)
		subscriberB = make(chan *domain.Backend, 100)

		backend1.SetHealthy()
		backend2.SetHealthy()
		backend3.SetHealthy()

		useLowestIndex = true
	})

	JustBeforeEach(func() {
		clusterMonitor = monitor.NewClusterMonitor(urlGetter, useTLSForAgent, backends, healthcheckTimeout, logger, useLowestIndex)
		clusterMonitor.RegisterBackendSubscriber(subscriberA)
		clusterMonitor.RegisterBackendSubscriber(subscriberB)
	})

	Describe("Monitor", func() {
		var (
			stopMonitoringChan chan interface{}
			backendToIndex     map[*domain.Backend]int
		)

		BeforeEach(func() {
			stopMonitoringChan = make(chan interface{})
			backendToIndex = map[*domain.Backend]int{
				backend1: 0,
				backend2: 1,
				backend3: 2,
			}

			useTLSForAgent := useTLSForAgent
			urlGetter.GetStub = func(url string) (*http.Response, error) {
				m.RLock()
				defer m.RUnlock()

				if url == backend1.HealthcheckUrls(useTLSForAgent)[0] {
					return healthyResponse(backendToIndex[backend1]), nil
				} else if url == backend2.HealthcheckUrls(useTLSForAgent)[0] {
					return healthyResponse(backendToIndex[backend2]), nil
				} else if url == backend3.HealthcheckUrls(useTLSForAgent)[0] {
					return healthyResponse(backendToIndex[backend3]), nil
				}

				panic("Unexpected backend")
			}
		})

		AfterEach(func() {
			close(stopMonitoringChan)
		})

		It("notices when each backend stays healthy", func() {
			backend1.SetUnhealthy()
			backend2.SetUnhealthy()
			backend3.SetUnhealthy()

			Expect(backend1.Healthy()).To(BeFalse())
			Expect(backend2.Healthy()).To(BeFalse())
			Expect(backend3.Healthy()).To(BeFalse())

			clusterMonitor.Monitor(stopMonitoringChan)

			Eventually(backend1.Healthy).Should(BeTrue())
			Eventually(backend2.Healthy).Should(BeTrue())
			Eventually(backend3.Healthy).Should(BeTrue())
		})

		It("notices when a healthy backend becomes unhealthy", func() {
			useTLSForAgent := useTLSForAgent
			urlGetter.GetStub = func(url string) (*http.Response, error) {
				m.RLock()
				defer m.RUnlock()

				if url == backend2.HealthcheckUrls(useTLSForAgent)[0] {
					return unhealthyResponse(0), nil
				} else {
					return healthyResponse(0), nil
				}
			}

			Expect(backend1.Healthy()).To(BeTrue())
			Expect(backend2.Healthy()).To(BeTrue())
			Expect(backend3.Healthy()).To(BeTrue())

			clusterMonitor.Monitor(stopMonitoringChan)

			Eventually(backend2.Healthy).Should(BeFalse())
			Consistently(backend1.Healthy).Should(BeTrue())
			Consistently(backend3.Healthy).Should(BeTrue())
		})

		It("notices when a healthy backend becomes unresponsive", func() {
			useTLSForAgent := useTLSForAgent
			urlGetter.GetStub = func(url string) (*http.Response, error) {
				m.RLock()
				defer m.RUnlock()
				if url == backend2.HealthcheckUrls(useTLSForAgent)[0] {
					return nil, errors.New("some error")
				} else {
					return healthyResponse(0), nil
				}
			}

			Expect(backend1.Healthy()).To(BeTrue())
			Expect(backend2.Healthy()).To(BeTrue())
			Expect(backend3.Healthy()).To(BeTrue())

			clusterMonitor.Monitor(stopMonitoringChan)

			Eventually(backend2.Healthy).Should(BeFalse())
			Consistently(backend1.Healthy).Should(BeTrue())
			Consistently(backend3.Healthy).Should(BeTrue())
		})

		It("notices when an unhealthy backend becomes healthy", func() {
			backend2.SetUnhealthy()

			useTLSForAgent := useTLSForAgent
			isUnhealthy := true
			urlGetter.GetStub = func(url string) (*http.Response, error) {
				m.RLock()
				defer m.RUnlock()
				if url == backend2.HealthcheckUrls(useTLSForAgent)[0] && isUnhealthy {
					isUnhealthy = false
					return unhealthyResponse(0), nil
				} else {
					return healthyResponse(0), nil
				}
			}

			Expect(backend1.Healthy()).To(BeTrue())
			Expect(backend2.Healthy()).To(BeFalse())
			Expect(backend3.Healthy()).To(BeTrue())

			clusterMonitor.Monitor(stopMonitoringChan)

			Eventually(backend2.Healthy).Should(BeTrue())
			Consistently(backend1.Healthy).Should(BeTrue())
			Consistently(backend3.Healthy).Should(BeTrue())
		})

		Context("when useLowestIndex is true", func() {
			Context("when the active backend changes", func() {
				It("publishes the new backend", func() {
					clusterMonitor.Monitor(stopMonitoringChan)

					Eventually(subscriberA).Should(Receive(Equal(backend1)))
					Eventually(subscriberB).Should(Receive(Equal(backend1)))

					m.Lock()
					backendToIndex = map[*domain.Backend]int{
						backend1: 1,
						backend2: 2,
						backend3: 0,
					}
					m.Unlock()

					Eventually(subscriberA).Should(Receive(Equal(backend3)))
					Eventually(subscriberB).Should(Receive(Equal(backend3)))
				})
			})
		})

		Context("when useLowestIndex is false", func() {
			BeforeEach(func() {
				useLowestIndex = false
			})

			Context("when the active backend changes", func() {
				It("publishes the new backend", func() {
					clusterMonitor.Monitor(stopMonitoringChan)

					Eventually(subscriberA).Should(Receive(Equal(backend3)))
					Eventually(subscriberB).Should(Receive(Equal(backend3)))
					m.Lock()
					backendToIndex = map[*domain.Backend]int{
						backend1: 0,
						backend2: 2,
						backend3: 1,
					}
					m.Unlock()

					Eventually(subscriberA).Should(Receive(Equal(backend2)))
					Eventually(subscriberB).Should(Receive(Equal(backend2)))
				})
			})
		})
	})

	Describe("QueryBackendHealth", func() {
		var (
			backend       *domain.Backend
			backendStatus *monitor.BackendStatus

			backendStatusPort uint
			backendHost       string
		)

		BeforeEach(func() {
			backendStatusPort = 9292
			backendHost = "192.0.2.10"

			backend = domain.NewBackend(
				"backend-0",
				backendHost,
				3306,
				backendStatusPort,
				"api/v1/status",
				logger,
			)
		})

		JustBeforeEach(func() {
			urlGetter.GetStub = func(url string) (*http.Response, error) {
				m.RLock()
				defer m.RUnlock()

				return healthyResponse(0), nil
			}

			backendStatus = &monitor.BackendStatus{
				Index:    2,
				Counters: clusterMonitor.SetupCounters(),
				Healthy:  false,
			}
		})

		It("changes the backend health and index", func() {
			Expect(backendStatus.Healthy).To(BeFalse())
			Expect(backendStatus.Index).To(Equal(2))

			clusterMonitor.QueryBackendHealth(backend, backendStatus)
			Expect(urlGetter.GetCallCount()).To(Equal(1))

			expectedURL := fmt.Sprintf(
				"http://%s:%d/api/v1/status",
				backendHost,
				backendStatusPort,
			)
			Expect(urlGetter.GetArgsForCall(0)).To(Equal(expectedURL))

			Expect(backendStatus.Healthy).To(BeTrue())
			Expect(backendStatus.Index).To(Equal(0))
		})

		When("TLS is enabled for Galera Agent communication", func() {
			BeforeEach(func() {
				useTLSForAgent = true
			})

			It("communicates with the agent over an encrypt channel", func() {
				clusterMonitor.QueryBackendHealth(backend, backendStatus)
				Expect(urlGetter.GetCallCount()).To(Equal(1))

				expectedURL := fmt.Sprintf(
					"https://%s:%d/api/v1/status",
					backendHost,
					backendStatusPort,
				)
				Expect(urlGetter.GetArgsForCall(0)).To(Equal(expectedURL))
			})

			When("our TLS connection is rejected by Galera Agent", func() {
				JustBeforeEach(func() {
					urlGetter.GetStub = func(url string) (*http.Response, error) {
						m.RLock()
						defer m.RUnlock()

						return &http.Response{
							Body:       ioutil.NopCloser(bytes.NewBuffer(nil)),
							StatusCode: http.StatusTeapot,
						}, errors.New("placeholder error")
					}
				})
				It("waterfalls through to a successful non-TLS connection", func() {
					expectedURL := fmt.Sprintf(
						"http://%s:%d/api/v1/status",
						backendHost,
						9200,
					)
					clusterMonitor.QueryBackendHealth(backend, backendStatus)
					Expect(urlGetter.GetCallCount()).To(Equal(2))
					Expect(urlGetter.GetArgsForCall(1)).To(Equal(expectedURL))
				})
			})
		})

		Context("when GETting the API returns an error", func() {
			JustBeforeEach(func() {
				urlGetter.GetStub = func(url string) (*http.Response, error) {
					m.RLock()
					defer m.RUnlock()
					return nil, errors.New("api not available")
				}
			})

			It("marks the backend as unhealthy", func() {
				backend.SetHealthy()

				clusterMonitor.QueryBackendHealth(backend, backendStatus)
				Expect(urlGetter.GetCallCount()).To(Equal(1))

				Expect(backendStatus.Healthy).To(BeFalse())
			})
		})

		Context("when GETting the API returns a bad status code", func() {
			JustBeforeEach(func() {
				urlGetter.GetStub = func(url string) (*http.Response, error) {
					m.RLock()
					defer m.RUnlock()

					return &http.Response{
						Body:       ioutil.NopCloser(bytes.NewBuffer(nil)),
						StatusCode: http.StatusTeapot,
					}, nil
				}
			})

			It("marks the backend as unhealthy", func() {
				backend.SetHealthy()

				clusterMonitor.QueryBackendHealth(backend, backendStatus)
				Expect(urlGetter.GetCallCount()).To(Equal(1))

				Expect(backendStatus.Healthy).To(BeFalse())
			})
		})
	})

	Describe("ChooseActiveBackend", func() {
		var (
			statuses                     map[*domain.Backend]*monitor.BackendStatus
			backend1, backend2, backend3 *domain.Backend
			useLowestIndex               bool
		)

		BeforeEach(func() {
			statuses = make(map[*domain.Backend]*monitor.BackendStatus)
			backend1 = domain.NewBackend(
				"backend-1",
				"10.10.1.2",
				1337,
				1338,
				"healthcheck",
				logger,
			)

			backend2 = domain.NewBackend(
				"backend-2",
				"10.10.2.2",
				1337,
				1338,
				"healthcheck",
				logger,
			)
			backend3 = domain.NewBackend(
				"backend-3",
				"10.10.3.2",
				1337,
				1338,
				"healthcheck",
				logger,
			)
			useLowestIndex = true
		})

		Context("When there are no backends", func() {
			It("returns nil", func() {
				Expect(monitor.ChooseActiveBackend(statuses, useLowestIndex)).To(BeNil())
			})
		})
		Context("If none of the backends are healthy", func() {
			It("returns nil", func() {
				statuses[backend1] = &monitor.BackendStatus{
					Healthy: false,
					Index:   0,
				}

				statuses[backend2] = &monitor.BackendStatus{
					Healthy: false,
					Index:   1,
				}

				statuses[backend3] = &monitor.BackendStatus{
					Healthy: false,
					Index:   2,
				}

				Expect(monitor.ChooseActiveBackend(statuses, useLowestIndex)).To(BeNil())
			})
		})

		Context("If only one of the backends is healthy", func() {
			It("chooses the only healthy one", func() {
				statuses[backend1] = &monitor.BackendStatus{
					Healthy: false,
					Index:   0,
				}

				statuses[backend2] = &monitor.BackendStatus{
					Healthy: false,
					Index:   1,
				}

				statuses[backend3] = &monitor.BackendStatus{
					Healthy: true,
					Index:   2,
				}

				Expect(monitor.ChooseActiveBackend(statuses, useLowestIndex)).To(Equal(backend3))
			})
		})

		Context("If multiple backends are healthy", func() {
			Context("when useLowestIndex is true", func() {
				It("chooses the healthy one with the lowest index", func() {
					statuses[backend2] = &monitor.BackendStatus{
						Healthy: true,
						Index:   2,
					}

					statuses[backend3] = &monitor.BackendStatus{
						Healthy: true,
						Index:   1,
					}

					statuses[backend1] = &monitor.BackendStatus{
						Healthy: false,
						Index:   0,
					}

					Expect(monitor.ChooseActiveBackend(statuses, useLowestIndex)).To(Equal(backend3))
				})
			})

			Context("when useLowestIndex is false", func() {
				BeforeEach(func() {
					useLowestIndex = false
				})

				It("chooses the healthy one with the highest index", func() {
					statuses[backend2] = &monitor.BackendStatus{
						Healthy: true,
						Index:   2,
					}

					statuses[backend3] = &monitor.BackendStatus{
						Healthy: true,
						Index:   1,
					}

					statuses[backend1] = &monitor.BackendStatus{
						Healthy: false,
						Index:   0,
					}

					Expect(monitor.ChooseActiveBackend(statuses, useLowestIndex)).To(Equal(backend2))
				})
			})
		})
	})
})

func healthyResponse(index int) *http.Response {
	healthyResponseBodyTemplate := `{"wsrep_local_state":4,"wsrep_local_state_comment":"Synced","wsrep_local_index":%d,"healthy":true}`

	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte(fmt.Sprintf(healthyResponseBodyTemplate, index)))),
		StatusCode: http.StatusOK,
	}
}

func unhealthyResponse(index int) *http.Response {
	unhealthyResponseBodyTemplate := `{"wsrep_local_state":2,"wsrep_local_state_comment":"Joiner","wsrep_local_index":%d,"healthy":false}`

	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte(fmt.Sprintf(unhealthyResponseBodyTemplate, index)))),
		StatusCode: http.StatusOK,
	}
}
