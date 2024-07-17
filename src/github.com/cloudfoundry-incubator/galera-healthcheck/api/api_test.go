package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/api/apifakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
)

const (
	ExpectedSeqno             = "4"
	ExpectedHealthCheckStatus = "synced"
	ApiUsername               = "fake-username"
	ApiPassword               = "fake-password"
)

var _ = Describe("Sidecar API", func() {
	var (
		monitClient      *apifakes.FakeMonitClient
		sequenceNumber   *apifakes.FakeSequenceNumberChecker
		healthchecker    *apifakes.FakeHealthChecker
		stateSnapshotter *apifakes.FakeStateSnapshotter
		ts               *httptest.Server

		ExpectedStateSnapshot domain.DBState
		testLogger            *lagertest.TestLogger
	)

	BeforeEach(func() {
		ExpectedStateSnapshot = domain.DBState{
			WsrepLocalIndex: 0,
			WsrepLocalState: domain.Synced,
			ReadOnly:        false,
		}

		monitClient = &apifakes.FakeMonitClient{}
		sequenceNumber = &apifakes.FakeSequenceNumberChecker{}
		sequenceNumber.CheckReturns(ExpectedSeqno, nil)

		healthchecker = &apifakes.FakeHealthChecker{}
		healthchecker.CheckReturns(ExpectedHealthCheckStatus, nil)

		stateSnapshotter = new(apifakes.FakeStateSnapshotter)
		stateSnapshotter.StateReturns(ExpectedStateSnapshot, nil)

		testLogger = lagertest.NewTestLogger("mysql_cmd")

		testConfig := &config.Config{
			SidecarEndpoint: config.SidecarEndpointConfig{
				Username: ApiUsername,
				Password: ApiPassword,
			},
			AvailableWhenReadOnly: true,
		}

		monitClient.StopServiceReturns("Successfully sent stop request", nil)
		monitClient.StartServiceBootstrapReturns("Successfully sent bootstrap request", nil)
		monitClient.StartServiceJoinReturns("Successfully sent join request", nil)
		monitClient.GetStatusReturns("running", nil)

		handler, err := api.NewRouter(
			testLogger,
			testConfig,
			monitClient,
			sequenceNumber,
			healthchecker,
			stateSnapshotter,
		)
		Expect(err).ToNot(HaveOccurred())
		ts = httptest.NewServer(handler)
	})

	AfterEach(func() {
		ts.Close()
	})

	Context("when request has basic auth", func() {

		var createReq = func(endpoint string, method string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest(method, url, nil)
			Expect(err).ToNot(HaveOccurred())

			req.SetBasicAuth(ApiUsername, ApiPassword)
			return req
		}

		It("Calls StopService on the monit client when a stop command is sent", func() {
			req := createReq("stop_mysql", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(monitClient.StopServiceCallCount()).To(Equal(1))
		})

		It("Calls StartService(join) on the monit client when a start command is sent in join mode", func() {
			req := createReq("start_mysql_join", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceJoinCallCount()).To(Equal(1))
		})

		It("Calls StartService(bootstrap) on the monit client when a start command is sent in bootstrap mode", func() {
			req := createReq("start_mysql_bootstrap", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceBootstrapCallCount()).To(Equal(1))
		})

		It("Calls StartService(single_node) on the monit client when a start command is sent in single_node mode", func() {
			req := createReq("start_mysql_single_node", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceSingleNodeCallCount()).To(Equal(1))
		})

		It("Calls GetStatus on the monit client when a new GetStatusCmd is created", func() {
			req := createReq("mysql_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.GetStatusCallCount()).To(Equal(1))
		})

		It("Calls Checker on the SequenceNumberchecker when a new sequence_number is created", func() {
			req := createReq("sequence_number", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(1))
		})

		It("returns 404 when a request is made to an unsupplied endpoint", func() {
			req := createReq("nonexistent_endpoint", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Context("when request does not have basic auth", func() {
		var createReq = func(endpoint string, method string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest(method, url, nil)
			Expect(err).ToNot(HaveOccurred())
			return req
		}

		It("requires authentication for /stop_mysql", func() {
			req := createReq("stop_mysql", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StopServiceCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_bootstrap", func() {
			req := createReq("start_mysql_bootstrap", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceBootstrapCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_join", func() {
			req := createReq("start_mysql_join", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceJoinCallCount()).To(Equal(0))
		})

		It("requires authentication for /mysql_status", func() {
			req := createReq("mysql_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.GetStatusCallCount()).To(Equal(0))
		})

		It("requires authentication for /sequence_number", func() {
			req := createReq("sequence_number", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).ToNot(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(0))
		})

		It("Calls Check on the reqHealthchecker at the root endpoint", func() {
			req := createReq("", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})

		It("Calls Check on the reqHealthchecker at /galera_status", func() {
			req := createReq("galera_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})

		Describe("/api/v1/status", func() {
			It("Calls State on the stateSnapshotter", func() {
				req := createReq("api/v1/status", "GET")
				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(stateSnapshotter.StateCallCount()).To(Equal(1))
			})

			Context("when getting the state succeeds", func() {
				var (
					returnedState domain.DBState
				)

				BeforeEach(func() {
					returnedState = domain.DBState{
						WsrepLocalIndex: 1,
						WsrepLocalState: domain.Synced,
						ReadOnly:        true,
					}

					stateSnapshotter.StateReturns(returnedState, nil)
				})

				It("has the required fields", func() {
					req := createReq("api/v1/status", "GET")
					resp, err := http.DefaultClient.Do(req)
					Expect(err).ToNot(HaveOccurred())

					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var state struct {
						WsrepLocalIndex        uint   `json:"wsrep_local_index"`
						WsrepLocalState        uint   `json:"wsrep_local_state"`
						WsrepLocalStateComment string `json:"wsrep_local_state_comment"`
						Healthy                bool   `json:"healthy"`
					}

					json.NewDecoder(resp.Body).Decode(&state)

					Expect(state.WsrepLocalIndex).To(Equal(returnedState.WsrepLocalIndex))
					Expect(state.WsrepLocalState).To(Equal(uint(returnedState.WsrepLocalState)))
					Expect(state.WsrepLocalStateComment).To(Equal(string(returnedState.WsrepLocalState.Comment())))
					Expect(state.Healthy).To(BeTrue())
				})

				It("logs the initial transition to its healthy state", func() {
					req := createReq("api/v1/status", "GET")
					resp, err := http.DefaultClient.Do(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					logData := testLogger.Logs()[0]
					Expect(logData.Message).To(Equal("mysql_cmd.health transition response: api.V1StatusResponse{WsrepLocalState:0x4, WsrepLocalStateComment:\"Synced\", WsrepLocalIndex:0x1, Healthy:true} maintenanceEnabled: false readOnly: true"))
				})

				When("a healthy node becomes & stays unhealthy", func() {
					BeforeEach(func() {
						stateSnapshotter.StateReturnsOnCall(0, domain.DBState{
							WsrepLocalIndex:    uint(2),
							WsrepLocalState:    domain.Synced,
							ReadOnly:           false,
							MaintenanceEnabled: false,
						}, nil)
						for i := 1; i <= 3; i++ {
							stateSnapshotter.StateReturnsOnCall(i, domain.DBState{
								WsrepLocalIndex:    uint(2),
								WsrepLocalState:    domain.Synced,
								ReadOnly:           false,
								MaintenanceEnabled: true, // trigger unhealthy state
							}, nil)
						}
					})

					It("logs a single transition to the unhealthy status", func() {
						req := createReq("api/v1/status", "GET")
						for i := 1; i <= 4; i++ {
							_, err := http.DefaultClient.Do(req)
							Expect(err).ToNot(HaveOccurred())
						}

						Expect(stateSnapshotter.StateCallCount()).To(Equal(4))
						Expect(len(testLogger.Logs())).To(Equal(2))
						logData := testLogger.Logs()[0] // initial "healthy" status
						Expect(logData.Message).To(Equal("mysql_cmd.health transition response: api.V1StatusResponse{WsrepLocalState:0x4, WsrepLocalStateComment:\"Synced\", WsrepLocalIndex:0x2, Healthy:true} maintenanceEnabled: false readOnly: false"))
						logData = testLogger.Logs()[1] // single "unhealthy" status
						Expect(logData.Message).To(Equal("mysql_cmd.health transition response: api.V1StatusResponse{WsrepLocalState:0x4, WsrepLocalStateComment:\"Synced\", WsrepLocalIndex:0x2, Healthy:false} maintenanceEnabled: true readOnly: false"))
					})
				})
				When("an unhealthy node becomes & stays healthy", func() {
					BeforeEach(func() {
						stateSnapshotter.StateReturnsOnCall(0, domain.DBState{
							WsrepLocalIndex:    uint(2),
							WsrepLocalState:    domain.Synced,
							ReadOnly:           false,
							MaintenanceEnabled: true, // triggers unhealthy state
						}, nil)
						for i := 1; i <= 3; i++ {
							stateSnapshotter.StateReturnsOnCall(i, domain.DBState{
								WsrepLocalIndex:    uint(2),
								WsrepLocalState:    domain.Synced,
								ReadOnly:           false,
								MaintenanceEnabled: false,
							}, nil)
						}
					})

					It("logs a single transition to the healthy status", func() {
						req := createReq("api/v1/status", "GET")
						for i := 1; i <= 4; i++ {
							_, err := http.DefaultClient.Do(req)
							Expect(err).ToNot(HaveOccurred())
						}

						Expect(stateSnapshotter.StateCallCount()).To(Equal(4))
						Expect(len(testLogger.Logs())).To(Equal(1))
						logData := testLogger.Logs()[0]
						Expect(logData.Message).To(Equal("mysql_cmd.health transition response: api.V1StatusResponse{WsrepLocalState:0x4, WsrepLocalStateComment:\"Synced\", WsrepLocalIndex:0x2, Healthy:true} maintenanceEnabled: false readOnly: false"))
					})
				})
			})

			Context("when getting the state fails", func() {
				BeforeEach(func() {
					stateSnapshotter.StateReturns(domain.DBState{}, errors.New("possibly not a galera cluster"))
				})

				It("500s", func() {
					req := createReq("api/v1/status", "GET")
					resp, err := http.DefaultClient.Do(req)
					Expect(err).ToNot(HaveOccurred())

					Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
				})

				It("logs the error", func() {
					req := createReq("api/v1/status", "GET")
					_, err := http.DefaultClient.Do(req)
					Expect(err).ToNot(HaveOccurred())

					Expect(len(testLogger.Logs())).To(Equal(1))
					logData := testLogger.Logs()[0]
					Expect(logData.Data["error"]).To(Equal("possibly not a galera cluster"))
				})
			})
		})
	})
})
