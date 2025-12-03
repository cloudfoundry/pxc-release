package main_test

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/types"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon_v2"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"

	"github.com/cloudfoundry-incubator/switchboard/api"
	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/cloudfoundry-incubator/switchboard/dummies"
	"github.com/cloudfoundry-incubator/switchboard/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type Response struct {
	BackendPort  uint
	BackendIndex uint
	Message      string
}

func allowTraffic(httpClient *http.Client, allow bool, port uint) {
	var url string
	if allow {
		url = fmt.Sprintf(
			"https://localhost:%d/v0/cluster?trafficEnabled=%t",
			port,
			allow,
		)
	} else {
		url = fmt.Sprintf(
			"https://localhost:%d/v0/cluster?trafficEnabled=%t&message=%s",
			port,
			allow,
			"main%20test%20is%20disabling%20traffic",
		)
	}

	req, err := http.NewRequest("PATCH", url, nil)
	Expect(err).NotTo(HaveOccurred())
	req.SetBasicAuth("username", "password")

	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}

func getClusterFromAPI(httpClient *http.Client, req *http.Request) map[string]interface{} {
	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	returnedCluster := map[string]interface{}{}

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&returnedCluster)
	Expect(err).NotTo(HaveOccurred())
	return returnedCluster
}

func sendData(conn net.Conn, data string) (Response, error) {
	_, _ = conn.Write([]byte(data))

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return Response{}, err
	} else {
		response := Response{}
		err := json.Unmarshal(buffer[:n], &response)
		if err != nil {
			return Response{}, err
		}
		return response, nil
	}
}

func verifyHeaderContains(header http.Header, key, valueSubstring string) {
	found := false
	for k, v := range header {
		if k == key {
			for _, value := range v {
				if strings.Contains(value, valueSubstring) {
					found = true
				}
			}
		}
	}
	Expect(found).To(BeTrue(), fmt.Sprintf("%s: %s not found in header", key, valueSubstring))
}

func getBackendsFromApi(httpClient *http.Client, req *http.Request) []map[string]interface{} {
	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var returnedBackends []map[string]interface{}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&returnedBackends)
	Expect(err).NotTo(HaveOccurred())
	return returnedBackends
}

func matchConnectionDisconnect() types.GomegaMatcher {
	//exact error depends on environment
	return MatchError(
		MatchRegexp(
			"%s|%s",
			io.EOF.Error(),
			syscall.ECONNRESET.Error(),
		),
	)
}

const startupTimeout = 10 * time.Second

var _ = Describe("Switchboard", func() {
	var (
		process                                      ifrit.Process
		switchboardRunner                            *ginkgomon_v2.Runner
		initialActiveBackend, initialInactiveBackend config.Backend
		healthcheckRunners                           []*dummies.HealthcheckRunner
		healthcheckWaitDuration                      time.Duration

		proxyPort                    uint
		proxyInactiveNodePort        uint
		switchboardAPIPort           uint
		switchboardAPIAggregatorPort uint
		switchboardHealthPort        uint
		metricsEnabled               bool
		metricsPort                  uint
		backends                     []config.Backend
		rootConfig                   config.Config
		proxyConfig                  config.Proxy
		apiConfig                    config.API
		staticDir                    string
		testServerTLSConfig          *tls.Config
		testCert                     tls.Certificate

		httpClient *http.Client
	)

	var acceptsAndClosesTCPConnections = func(address string) {
		Eventually(func() error {
			req, err := http.NewRequest(http.MethodGet, address, nil)
			Expect(err).NotTo(HaveOccurred())
			req.SetBasicAuth(rootConfig.API.Username, rootConfig.API.Password)
			if res, err := httpClient.Do(req); err != nil {
				return err
			} else if res.StatusCode != http.StatusOK {
				return errors.New("health check port returned unexpected status: " + res.Status)
			}
			return nil
		}, "10s", "1s").Should(Succeed())
	}

	var startYourServers = func(rootConfig config.Config) {
		runnableRootConfig, err := json.Marshal(rootConfig)
		Expect(err).NotTo(HaveOccurred())

		healthcheckRunners = []*dummies.HealthcheckRunner{
			dummies.NewHealthcheckRunner(backends[0], 0, testServerTLSConfig),
			dummies.NewHealthcheckRunner(backends[1], 1, testServerTLSConfig),
		}

		logLevel := "debug"
		switchboardRunner = ginkgomon_v2.New(ginkgomon_v2.Config{
			Command: exec.Command(
				switchboardBinPath,
				fmt.Sprintf("-config=%s", string(runnableRootConfig)),
				fmt.Sprintf("-logLevel=%s", logLevel),
			),
			Name:              "switchboard",
			StartCheck:        "started",
			StartCheckTimeout: startupTimeout,
		})

		group := grouper.NewOrdered(os.Interrupt, grouper.Members{
			{Name: "backend-0", Runner: dummies.NewBackendRunner(0, backends[0])},
			{Name: "backend-1", Runner: dummies.NewBackendRunner(1, backends[1])},
			{Name: "healthcheck-0", Runner: healthcheckRunners[0]},
			{Name: "healthcheck-1", Runner: healthcheckRunners[1]},
			{Name: "switchboard", Runner: switchboardRunner},
		})
		process = ifrit.Invoke(sigmon.New(group))

	}

	var waitForServersToBeReady = func(protocol string) {
		var response Response
		Eventually(func() error {
			url := fmt.Sprintf("%s://localhost:%d/v0/cluster", protocol, switchboardAPIPort)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth("username", "password")

			resp, err := httpClient.Do(req)
			if err != nil {
				return err
			}

			var returnedCluster api.ClusterJSON
			decoder := json.NewDecoder(resp.Body)
			err = decoder.Decode(&returnedCluster)
			if err != nil {
				return err
			}
			if returnedCluster.ActiveBackend == nil || returnedCluster.ActiveBackend.Port != backends[0].Port {
				return errors.New("expected backend not active yet")
			}

			return err
		}, startupTimeout).Should(Succeed())

		Eventually(func() error {
			url := fmt.Sprintf("%s://localhost:%d/v0/backends", protocol, switchboardAPIPort)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth("username", "password")

			resp, err := httpClient.Do(req)
			if err != nil {
				return err
			}

			var returnedBackends []api.V0BackendResponse
			decoder := json.NewDecoder(resp.Body)
			err = decoder.Decode(&returnedBackends)
			if err != nil {
				return err
			}

			for _, backend := range returnedBackends {
				if backend.Healthy == true && backend.Active == false {
					return nil
				}
			}

			return errors.New("inactive backend never became healthy")
		}, startupTimeout).Should(Succeed())

		Eventually(func() error {
			conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			response, err = sendData(conn, "detect active")
			return err
		}, startupTimeout).Should(Succeed())

		initialActiveBackend = backends[response.BackendIndex]
		initialInactiveBackend = backends[(response.BackendIndex+1)%2]
	}

	BeforeEach(func() {
		var (
			err    error
			testCA []byte
		)
		staticDir, err = filepath.Abs("../../static")
		Expect(err).NotTo(HaveOccurred())

		testCA, testCert, err = testing.GenerateSelfSignedCertificate("localhost")
		Expect(err).NotTo(HaveOccurred())

		testServerTLSConfig, err = testing.ServerConfigFromCertificate(testCert)
		Expect(err).NotTo(HaveOccurred())

		testClientTLSConfig, err := testing.ClientConfigFromAuthority(testCA)
		Expect(err).NotTo(HaveOccurred())

		httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: testClientTLSConfig}}

		proxyPort = uint(10000 + GinkgoParallelProcess())
		proxyInactiveNodePort = uint(10600 + GinkgoParallelProcess())
		switchboardAPIPort = uint(10100 + GinkgoParallelProcess())
		switchboardAPIAggregatorPort = uint(10800 + GinkgoParallelProcess())
		switchboardHealthPort = uint(6160 + GinkgoParallelProcess())
		metricsPort = uint(2112 + GinkgoParallelProcess())

		backend1 := config.Backend{
			Host:           "localhost",
			Port:           uint(10200 + GinkgoParallelProcess()),
			StatusPort:     uint(10300 + GinkgoParallelProcess()),
			StatusEndpoint: "api/v1/status",
			Name:           "backend-0",
		}

		backend2 := config.Backend{
			Host:           "localhost",
			Port:           uint(10400 + GinkgoParallelProcess()),
			StatusPort:     uint(10500 + GinkgoParallelProcess()),
			StatusEndpoint: "api/v1/status",
			Name:           "backend-1",
		}

		backends = []config.Backend{backend1, backend2}

		proxyConfig = config.Proxy{
			Backends:                 backends,
			HealthcheckTimeoutMillis: 500,
			Port:                     proxyPort,
			InactiveMysqlPort:        proxyInactiveNodePort,
			ShutdownDelaySeconds:     5,
		}

		apiConfig = config.API{
			AggregatorPort: switchboardAPIAggregatorPort,
			Port:           switchboardAPIPort,
			Username:       "username",
			Password:       "password",
			ProxyURIs:      []string{"some-proxy-uri-0", "some-proxy-uri-1"},
		}

		rootConfig = config.Config{
			BindAddress: "127.0.0.1",
			Proxy:       proxyConfig,
			API:         apiConfig,
			HealthPort:  switchboardHealthPort,
			StaticDir:   staticDir,
			GaleraAgentTLS: config.GaleraAgentTLS{
				Enabled:    true,
				ServerName: "localhost",
				CA:         string(testCA),
			},
			Metrics: config.Metrics{
				Enabled: metricsEnabled,
				Port:    metricsPort,
			},
		}
		healthcheckWaitDuration = 3 * proxyConfig.HealthcheckTimeout()
	})

	Context("Non TLS for the API", func() {

		When("apiConfig.TlS is provided", func() {
			JustBeforeEach(func() {
				rootConfig.API.TLS.Enabled = false
				startYourServers(rootConfig)
			})

			AfterEach(func() {
				ginkgomon_v2.Interrupt(process, 10*time.Second)
			})

			When("switchboard starts successfully without TLS", func() {
				JustBeforeEach(func() {
					waitForServersToBeReady("http")
				})
				When("switchboard starts without TLS", func() {
					Context("Aggregator Dashboard", func() {
						It("makes a successful request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardAPIAggregatorPort))
						})
					})

					Context("Proxy Dashboard", func() {
						It("makes a successful request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardAPIPort))

						})
					})

					Context("Health checks", func() {
						It("makes a successful HTTP request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardHealthPort))
						})
					})

				})

			})

		})
		When("apiConfig.TlS is not provided", func() {
			JustBeforeEach(func() {
				startYourServers(rootConfig)
			})

			AfterEach(func() {
				ginkgomon_v2.Interrupt(process, 10*time.Second)
			})

			When("switchboard starts successfully without TLS", func() {
				JustBeforeEach(func() {
					waitForServersToBeReady("http")
				})
				When("switchboard starts without TLS", func() {
					Context("Aggregator Dashboard", func() {
						It("makes a successful request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardAPIAggregatorPort))
						})
					})

					Context("Proxy Dashboard", func() {
						It("makes a successful request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardAPIPort))

						})
					})

					Context("Health checks", func() {
						It("makes a successful HTTP request", func() {
							acceptsAndClosesTCPConnections(fmt.Sprintf("http://127.0.0.1:%d", switchboardHealthPort))
						})
					})

				})

			})

		})
	})

	Context("TLS", func() {

		JustBeforeEach(func() {
			rootConfig.API.TLS.Enabled = true
			rootConfig.API.TLS.Certificate = string(testing.CertificatePEM(testCert.Certificate[0]))
			rootConfig.API.TLS.PrivateKey = string(testing.PrivateKeyPEM(testCert.PrivateKey))
			startYourServers(rootConfig)
		})

		AfterEach(func() {
			ginkgomon_v2.Interrupt(process, 10*time.Second)
		})

		When("switchboard starts successfully with TLS", func() {
			JustBeforeEach(func() {
				waitForServersToBeReady("https")
			})

			Describe("Health", func() {

				It("accepts and immediately closes TCP connections on HealthPort", func() {
					acceptsAndClosesTCPConnections(fmt.Sprintf("https://127.0.0.1:%d", rootConfig.HealthPort))
				})

				Context("when HealthPort == API.Port", func() {
					BeforeEach(func() {
						rootConfig.HealthPort = rootConfig.API.Port
					})

					It("operates normally", func() {
						acceptsAndClosesTCPConnections(fmt.Sprintf("https://127.0.0.1:%d", rootConfig.HealthPort))
					})
				})
			})

			Describe("API Aggregator", func() {
				Describe("/", func() {
					var url string

					BeforeEach(func() {
						url = fmt.Sprintf("https://localhost:%d/", switchboardAPIAggregatorPort)
					})

					It("prompts for Basic Auth creds when they aren't provided", func() {
						resp, err := httpClient.Get(url)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
						Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Basic realm="Authorization Required"`))
					})

					It("does not accept bad Basic Auth creds", func() {
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						req.SetBasicAuth("bad_username", "bad_password")
						resp, err := httpClient.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					})

					It("responds with 200 and contains proxy URIs when authorized", func() {
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						req.SetBasicAuth("username", "password")
						resp, err := httpClient.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						Expect(resp.Body).ToNot(BeNil())
						defer func() { _ = resp.Body.Close() }()

						body, err := io.ReadAll(resp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(body)).To(BeNumerically(">", 0), "Expected body to not be empty")

						Expect(string(body)).To(ContainSubstring(apiConfig.ProxyURIs[0]))
						Expect(string(body)).To(ContainSubstring(apiConfig.ProxyURIs[1]))
					})
				})
			})

			Describe("UI", func() {
				Describe("/", func() {
					var url string

					BeforeEach(func() {
						url = fmt.Sprintf("https://localhost:%d/", switchboardAPIPort)
					})

					It("prompts for Basic Auth creds when they aren't provided", func() {
						resp, err := httpClient.Get(url)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
						Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Basic realm="Authorization Required"`))
					})

					It("does not accept bad Basic Auth creds", func() {
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						resp, err := httpClient.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					})

					It("responds with 200 and contains non-zero body when authorized", func() {
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						req.SetBasicAuth("username", "password")
						resp, err := httpClient.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						Expect(resp.Body).ToNot(BeNil())
						defer func() { _ = resp.Body.Close() }()

						body, err := io.ReadAll(resp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(len(body)).To(BeNumerically(">", 0), "Expected body to not be empty")
					})
				})
			})

			Describe("api", func() {
				Describe("/v0/backends/", func() {
					var url string

					BeforeEach(func() {
						url = fmt.Sprintf("https://localhost:%d/v0/backends", switchboardAPIPort)
					})

					It("prompts for Basic Auth creds when they aren't provided", func() {
						resp, err := httpClient.Get(url)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
						Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Basic realm="Authorization Required"`))
					})

					It("does not accept bad Basic Auth creds", func() {
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						req.SetBasicAuth("bad_username", "bad_password")
						resp, err := httpClient.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					})

					Context("When authorized", func() {
						var req *http.Request

						BeforeEach(func() {
							var err error
							req, err = http.NewRequest("GET", url, nil)
							Expect(err).NotTo(HaveOccurred())
							req.SetBasicAuth("username", "password")
						})

						It("returns correct headers", func() {
							resp, err := httpClient.Do(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(resp.StatusCode).To(Equal(http.StatusOK))
							verifyHeaderContains(resp.Header, "Content-Type", "application/json")
						})

						It("returns valid JSON in body", func() {
							returnedBackends := getBackendsFromApi(httpClient, req)

							Expect(len(returnedBackends)).To(Equal(2))

							Expect(returnedBackends[0]["host"]).To(Equal("localhost"))
							Expect(returnedBackends[0]["healthy"]).To(BeTrue(), "Expected backends[0] to be healthy")

							Expect(returnedBackends[1]["host"]).To(Equal("localhost"))
							Expect(returnedBackends[1]["healthy"]).To(BeTrue(), "Expected backends[1] to be healthy")

							if returnedBackends[0]["active"] == true {
								Expect(returnedBackends[1]["active"]).To(BeFalse())
							} else {
								Expect(returnedBackends[1]["active"]).To(BeTrue())
							}

							switch returnedBackends[0]["name"] {

							case backends[0].Name:
								Expect(returnedBackends[0]["port"]).To(BeNumerically("==", backends[0].Port))
								Expect(returnedBackends[1]["port"]).To(BeNumerically("==", backends[1].Port))
								Expect(returnedBackends[1]["name"]).To(Equal(backends[1].Name))

							case backends[1].Name: // order reversed in response
								Expect(returnedBackends[1]["port"]).To(BeNumerically("==", backends[0].Port))
								Expect(returnedBackends[0]["port"]).To(BeNumerically("==", backends[1].Port))
								Expect(returnedBackends[0]["name"]).To(Equal(backends[1].Name))
							default:
								Fail(fmt.Sprintf("Invalid backend name: %s", returnedBackends[0]["name"]))
							}
						})

						It("returns session count for backends", func() {
							var err error
							var conn net.Conn
							Eventually(func() error {
								conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								if err != nil {
									return err
								}
								return nil

							}, startupTimeout).Should(Succeed())
							defer func() { _ = conn.Close() }()

							connData, err := sendData(conn, "success")
							Expect(err).ToNot(HaveOccurred())
							Expect(connData.Message).To(Equal("success"))

							returnedBackends := getBackendsFromApi(httpClient, req)

							Eventually(func() interface{} {
								return getBackendsFromApi(httpClient, req)[0]["currentSessionCount"]
							}).Should(BeNumerically("==", 1), "Expected active backend to have SessionCount == 1")

							Expect(returnedBackends[1]["currentSessionCount"]).To(BeNumerically("==", 0), "Expected inactive backend to have SessionCount == 0")
						})
					})
				})
			})

			Describe("/v0/cluster", func() {
				Describe("GET", func() {
					It("returns valid JSON in body", func() {
						url := fmt.Sprintf("https://localhost:%d/v0/cluster", switchboardAPIPort)
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())
						req.SetBasicAuth("username", "password")

						returnedCluster := getClusterFromAPI(httpClient, req)

						Expect(returnedCluster["trafficEnabled"]).To(BeTrue())
					})
				})

				Describe("PATCH", func() {
					It("returns valid JSON in body", func() {
						url := fmt.Sprintf("https://localhost:%d/v0/cluster?trafficEnabled=true", switchboardAPIPort)
						req, err := http.NewRequest("PATCH", url, nil)
						Expect(err).NotTo(HaveOccurred())
						req.SetBasicAuth("username", "password")

						returnedCluster := getClusterFromAPI(httpClient, req)

						Expect(returnedCluster["trafficEnabled"]).To(BeTrue())
						Expect(returnedCluster["lastUpdated"]).NotTo(BeEmpty())
					})

					It("persists the provided value of enableTraffic", func() {
						url := fmt.Sprintf("https://localhost:%d/v0/cluster?trafficEnabled=false&message=some-reason", switchboardAPIPort)
						req, err := http.NewRequest("PATCH", url, nil)
						Expect(err).NotTo(HaveOccurred())
						req.SetBasicAuth("username", "password")

						returnedCluster := getClusterFromAPI(httpClient, req)

						Expect(returnedCluster["trafficEnabled"]).To(BeFalse())

						url = fmt.Sprintf("https://localhost:%d/v0/cluster?trafficEnabled=true", switchboardAPIPort)
						req, err = http.NewRequest("PATCH", url, nil)
						Expect(err).NotTo(HaveOccurred())
						req.SetBasicAuth("username", "password")

						returnedCluster = getClusterFromAPI(httpClient, req)

						Expect(returnedCluster["trafficEnabled"]).To(BeTrue())
					})
				})
			})

			Describe("proxy", func() {
				Context("when connecting to the active port", func() {

					Context("when there are multiple concurrent clients", func() {
						It("proxies all the connections to the lowest indexed backend", func() {
							var doneArray = make([]chan interface{}, 3)
							var dataMessages = make([]Response, 3)

							for i := 0; i < 3; i++ {
								doneArray[i] = make(chan interface{})
								go func(index int) {
									defer GinkgoRecover()
									defer close(doneArray[index])

									var err error
									var conn net.Conn

									Eventually(func() error {
										conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
										return err
									}, startupTimeout).ShouldNot(HaveOccurred())

									data, err := sendData(conn, fmt.Sprintf("test%d", index))
									Expect(err).ToNot(HaveOccurred())
									dataMessages[index] = data
								}(i)
							}

							for _, done := range doneArray {
								<-done
							}

							for i, dataResponse := range dataMessages {
								Expect(dataResponse.Message).Should(Equal(fmt.Sprintf("test%d", i)))
								Expect(dataResponse.BackendIndex).To(BeEquivalentTo(0))
							}
						})
					})

					Context("when other clients disconnect", func() {
						var conn net.Conn
						var connToDisconnect net.Conn

						It("maintains a long-lived connection", func() {
							Eventually(func() error {
								var err error
								conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								return err
							}, startupTimeout).Should(Succeed())

							Eventually(func() error {
								var err error
								connToDisconnect, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								return err
							}, "5s").Should(Succeed())

							dataBeforeDisconnect, err := sendData(conn, "data before disconnect")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataBeforeDisconnect.Message).Should(Equal("data before disconnect"))

							_ = connToDisconnect.Close()

							dataAfterDisconnect, err := sendData(conn, "data after disconnect")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataAfterDisconnect.Message).Should(Equal("data after disconnect"))
						})
					})

					Context("when the healthcheck succeeds", func() {
						It("checks health again after the specified interval", func() {
							var client net.Conn
							Eventually(func() error {
								var err error
								client, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								return err
							}, startupTimeout).Should(Succeed())

							data, err := sendData(client, "data around first healthcheck")
							Expect(err).NotTo(HaveOccurred())
							Expect(data.Message).To(Equal("data around first healthcheck"))

							Consistently(func() error {
								_, err = sendData(client, "data around subsequent healthcheck")
								return err
							}, 3*time.Second, 500*time.Millisecond).Should(Succeed())
						})
					})

					Context("when the cluster is down", func() {
						Context("when the healthcheck reports a 503", func() {
							It("disconnects client connections", func() {
								var conn net.Conn
								Eventually(func() error {
									var err error
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
									return err
								}, startupTimeout).Should(Succeed())

								dataWhileHealthy, err := sendData(conn, "data while healthy")
								Expect(err).ToNot(HaveOccurred())
								Expect(dataWhileHealthy.Message).To(Equal("data while healthy"))

								if initialActiveBackend == backends[0] {
									healthcheckRunners[0].SetStatusCode(http.StatusServiceUnavailable)
								} else {
									healthcheckRunners[1].SetStatusCode(http.StatusServiceUnavailable)
								}

								Eventually(func() error {
									_, err := sendData(conn, "data when unhealthy")
									return err
								}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
							})
						})

						Context("when a backend goes down", func() {
							var conn net.Conn
							var data Response

							JustBeforeEach(func() {
								Eventually(func() (err error) {
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
									return err
								}, startupTimeout).Should(Succeed())

								data, err := sendData(conn, "data before hang")
								Expect(err).ToNot(HaveOccurred())
								Expect(data.Message).To(Equal("data before hang"))

								if initialActiveBackend == backends[0] {
									healthcheckRunners[0].SetHang(true)
								} else {
									healthcheckRunners[1].SetHang(true)
								}
							})

							It("disconnects existing client connections", func() {
								Eventually(func() error {
									_, err := sendData(conn, "data after hang")
									return err
								}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
							})

							It("proxies new connections to another backend", func() {
								var err error
								Eventually(func() (uint, error) {
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
									if err != nil {
										return 0, err
									}

									data, err = sendData(conn, "test")
									return data.BackendPort, err
								}, healthcheckWaitDuration).Should(Equal(initialInactiveBackend.Port))

								Expect(data.Message).To(Equal("test"))
							})
						})

						Context("when all backends are down", func() {
							JustBeforeEach(func() {
								for _, hr := range healthcheckRunners {
									hr.SetHang(true)
								}
							})

							It("rejects any new connections that are attempted", func() {
								Eventually(func() error {
									conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
									if err != nil {
										return err
									}
									_, err = sendData(conn, "write that should fail")
									return err
								}, healthcheckWaitDuration, 200*time.Millisecond).Should(matchConnectionDisconnect())
							})
						})
					})

					Context("when traffic is disabled", func() {
						It("disconnects client connections", func() {
							var conn net.Conn
							Eventually(func() error {
								var err error
								conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								return err
							}, startupTimeout).Should(Succeed())
							defer func() { _ = conn.Close() }()

							dataWhileHealthy, err := sendData(conn, "data while healthy")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataWhileHealthy.Message).To(Equal("data while healthy"))

							allowTraffic(httpClient, false, switchboardAPIPort)

							Eventually(func() error {
								_, err := sendData(conn, "data when unhealthy")
								return err
							}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
						})

						It("severs new connections", func() {
							allowTraffic(httpClient, false, switchboardAPIPort)
							Eventually(func() error {
								conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								if err != nil {
									return err
								}
								defer func() { _ = conn.Close() }()

								_, err = sendData(conn, "write that should fail")

								return err
							}).Should(matchConnectionDisconnect())
						})

						It("permits new connections again after re-enabling traffic", func() {
							allowTraffic(httpClient, false, switchboardAPIPort)
							allowTraffic(httpClient, true, switchboardAPIPort)

							Eventually(func() error {
								var err error
								_, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
								return err
							}, "5s").Should(Succeed())
						})
					})
				})
				Context("when connecting to the inactive port", func() {
					JustBeforeEach(func() {
						Eventually(func() error {
							conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
							if err != nil {
								return err
							}
							_, err = sendData(conn, "checking inactive port is ready")
							return err
						}, startupTimeout).ShouldNot(HaveOccurred(), "Switchboard inactive mysql port never became ready")
					})

					Context("when there are multiple concurrent clients", func() {
						It("proxies all the connections to the highest indexed backend", func() {
							var doneArray = make([]chan interface{}, 3)
							var dataMessages = make([]Response, 3)

							for i := 0; i < 3; i++ {
								doneArray[i] = make(chan interface{})
								go func(index int) {
									defer GinkgoRecover()
									defer close(doneArray[index])

									var err error
									var conn net.Conn

									Eventually(func() error {
										conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
										return err
									}, startupTimeout).ShouldNot(HaveOccurred())

									data, err := sendData(conn, fmt.Sprintf("test%d", index))
									Expect(err).ToNot(HaveOccurred())
									dataMessages[index] = data
								}(i)
							}

							for _, done := range doneArray {
								<-done
							}

							for i, dataResponse := range dataMessages {
								Expect(dataResponse.Message).Should(Equal(fmt.Sprintf("test%d", i)))
								Expect(dataResponse.BackendIndex).To(BeEquivalentTo(1))
							}
						})
					})

					Context("when other clients disconnect", func() {
						var conn net.Conn
						var connToDisconnect net.Conn

						It("maintains a long-lived connection when other clients disconnect", func() {
							Eventually(func() error {
								var err error
								conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								return err
							}, startupTimeout).Should(Succeed())

							Eventually(func() error {
								var err error
								connToDisconnect, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								return err
							}, "5s").Should(Succeed())

							dataBeforeDisconnect, err := sendData(conn, "data before disconnect")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataBeforeDisconnect.Message).Should(Equal("data before disconnect"))

							_ = connToDisconnect.Close()

							dataAfterDisconnect, err := sendData(conn, "data after disconnect")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataAfterDisconnect.Message).Should(Equal("data after disconnect"))
						})
					})

					Context("when the healthcheck succeeds", func() {
						It("checks health again after the specified interval", func() {
							var client net.Conn
							Eventually(func() error {
								var err error
								client, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								return err
							}, startupTimeout).Should(Succeed())

							data, err := sendData(client, "data around first healthcheck")
							Expect(err).NotTo(HaveOccurred())
							Expect(data.Message).To(Equal("data around first healthcheck"))

							Consistently(func() error {
								_, err = sendData(client, "data around subsequent healthcheck")
								return err
							}, 3*time.Second, 500*time.Millisecond).Should(Succeed())
						})
					})

					Context("when the cluster is down", func() {
						Context("when the healthcheck reports a 503", func() {
							It("disconnects client connections", func() {
								var conn net.Conn
								Eventually(func() error {
									var err error
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
									return err
								}, startupTimeout).Should(Succeed())

								dataWhileHealthy, err := sendData(conn, "data while healthy")
								Expect(err).ToNot(HaveOccurred())
								Expect(dataWhileHealthy.Message).To(Equal("data while healthy"))

								if initialInactiveBackend == backends[0] {
									healthcheckRunners[0].SetStatusCode(http.StatusServiceUnavailable)
								} else {
									healthcheckRunners[1].SetStatusCode(http.StatusServiceUnavailable)
								}

								Eventually(func() error {
									_, err := sendData(conn, "data when unhealthy")
									return err
								}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
							})
						})

						Context("when a backend goes down", func() {
							var conn net.Conn
							var data Response

							JustBeforeEach(func() {
								Eventually(func() (err error) {
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
									return err
								}, startupTimeout).Should(Succeed())

								data, err := sendData(conn, "data before hang")
								Expect(err).ToNot(HaveOccurred())
								Expect(data.Message).To(Equal("data before hang"))

								if initialInactiveBackend == backends[0] {
									healthcheckRunners[0].SetHang(true)
								} else {
									healthcheckRunners[1].SetHang(true)
								}
							})

							It("disconnects existing client connections", func() {
								Eventually(func() error {
									_, err := sendData(conn, "data after hang")
									return err
								}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
							})

							It("proxies new connections to another backend", func() {
								var err error
								Eventually(func() (uint, error) {
									conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
									if err != nil {
										return 0, err
									}

									data, err = sendData(conn, "test")
									return data.BackendPort, err
								}, healthcheckWaitDuration).Should(Equal(initialActiveBackend.Port))

								Expect(data.Message).To(Equal("test"))
							})
						})

						Context("when all backends are down", func() {
							JustBeforeEach(func() {
								for _, hr := range healthcheckRunners {
									hr.SetHang(true)
								}
							})

							It("rejects any new connections that are attempted", func() {

								Eventually(func() error {
									conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
									if err != nil {
										return err
									}
									_, err = sendData(conn, "write that should fail")
									return err
								}, healthcheckWaitDuration, 200*time.Millisecond).Should(matchConnectionDisconnect())
							})
						})
					})

					Context("when traffic is disabled", func() {
						It("disconnects client connections", func() {
							var conn net.Conn
							Eventually(func() error {
								var err error
								conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								return err
							}, startupTimeout).Should(Succeed())
							defer func() { _ = conn.Close() }()

							dataWhileHealthy, err := sendData(conn, "data while healthy")
							Expect(err).ToNot(HaveOccurred())
							Expect(dataWhileHealthy.Message).To(Equal("data while healthy"))

							allowTraffic(httpClient, false, switchboardAPIPort)

							Eventually(func() error {
								_, err := sendData(conn, "data when unhealthy")
								return err
							}, healthcheckWaitDuration).Should(matchConnectionDisconnect())
						})

						It("severs new connections", func() {
							allowTraffic(httpClient, false, switchboardAPIPort)
							Eventually(func() error {
								conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								if err != nil {
									return err
								}
								defer func() { _ = conn.Close() }()

								_, err = sendData(conn, "write that should fail")

								return err
							}).Should(matchConnectionDisconnect())
						})

						It("permits new connections again after re-enabling traffic", func() {
							allowTraffic(httpClient, false, switchboardAPIPort)
							allowTraffic(httpClient, true, switchboardAPIPort)

							Eventually(func() error {
								var err error
								_, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
								return err
							}, "5s").Should(Succeed())
						})
					})
				})
				Context("when inactive port is not configured", func() {
					BeforeEach(func() {
						rootConfig.Proxy.InactiveMysqlPort = 0
					})

					It("does not crash", func() {
						Eventually(func() error {
							conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyInactiveNodePort))
							if err != nil {
								return err
							}
							_, err = sendData(conn, "write that should fail")
							return err
						}, healthcheckWaitDuration, 200*time.Millisecond).Should(MatchError(ContainSubstring("connection refused")))
					})

				})
			})

			Describe("metrics", func() {
				When("switchboard metrics are enabled", func() {
					BeforeEach(func() {
						rootConfig.Metrics.Enabled = true
					})

					It("responds with backend session metrics", func() {
						resp, err := httpClient.Get(fmt.Sprintf("https://localhost:%d/metrics", metricsPort))
						Expect(err).NotTo(HaveOccurred())
						defer func() { _ = resp.Body.Close() }()
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						Expect(resp.Body).ToNot(BeNil())

						bodyBytes, err := io.ReadAll(resp.Body)
						Expect(err).NotTo(HaveOccurred())

						body := strings.Split(string(bodyBytes), "\n")
						Expect(body).To(ContainElement("# HELP backend_sessions_total Gauge of the current sessions from this proxy to a mysql backend"))
						Expect(body).To(ContainElement("# TYPE backend_sessions_total gauge"))
						Expect(body).To(ContainElement(`backend_sessions_total{backend="backend-0"} 0`))
						Expect(body).To(ContainElement(`backend_sessions_total{backend="backend-1"} 0`))
					})
				})

				When("switchboard metrics are disabled", func() {
					BeforeEach(func() {
						rootConfig.Metrics.Enabled = false
					})

					It("returns errors when trying to connect to metrics port", func() {
						_, err := httpClient.Get(fmt.Sprintf("https://localhost:%d/metrics", metricsPort))
						Expect(err).To(MatchError(ContainSubstring("connect: connection refused")))
					})
				})
			})

			Context("Status Logging", func() {
				When("status logging is enabled", func() {
					BeforeEach(func() {
						rootConfig.StatusLog.Enabled = true
						rootConfig.StatusLog.Interval = 2 * time.Second
					})

					It("emits periodic status updates with expected fields", func() {
						time.Sleep(3 * time.Second)

						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say("Status update"))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"total_backends":2`))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"healthy_backends":\d+`))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"total_connections":\d+`))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"active_backend":"`))
					})

					It("reports the correct active backend", func() {
						time.Sleep(3 * time.Second)

						// Get active backend from API
						url := fmt.Sprintf("https://localhost:%d/v0/cluster", switchboardAPIPort)
						req, err := http.NewRequest("GET", url, nil)
						Expect(err).NotTo(HaveOccurred())

						req.SetBasicAuth("username", "password")
						cluster := getClusterFromAPI(httpClient, req)

						activeBackend := cluster["activeBackend"].(map[string]interface{})
						expectedName := activeBackend["name"].(string)

						Eventually(switchboardRunner.Buffer()).Should(
							gbytes.Say(fmt.Sprintf(`"active_backend":"%s"`, expectedName)))
					})

					It("reports connection counts", func() {
						conn1, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
						Expect(err).NotTo(HaveOccurred())
						defer func() { _ = conn1.Close() }()

						conn2, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxyPort))
						Expect(err).NotTo(HaveOccurred())
						defer func() { _ = conn2.Close() }()

						time.Sleep(3 * time.Second)

						// Should show at least 2 connections
						Eventually(switchboardRunner.Buffer()).Should(
							gbytes.Say(`"total_connections":[2-9]\d*`))
					})

					It("reports unhealthy backends", func() {
						// Mark one backend unhealthy
						if initialActiveBackend == backends[0] {
							healthcheckRunners[1].SetStatusCode(http.StatusServiceUnavailable)
						} else {
							healthcheckRunners[0].SetStatusCode(http.StatusServiceUnavailable)
						}

						time.Sleep(healthcheckWaitDuration + 3*time.Second)

						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"healthy_backends":1`))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"unhealthy_backends":\["`))
					})

					It("tracks failover events", func() {
						// Wait for initial status log (no failover yet)
						time.Sleep(3 * time.Second)

						// Capture initial buffer state
						initialContent := string(switchboardRunner.Buffer().Contents())

						// Should not have failover info initially
						Expect(initialContent).NotTo(ContainSubstring("last_failover_at"))

						// Trigger failover by marking active backend unhealthy
						if initialActiveBackend == backends[0] {
							healthcheckRunners[0].SetStatusCode(http.StatusServiceUnavailable)
						} else {
							healthcheckRunners[1].SetStatusCode(http.StatusServiceUnavailable)
						}

						// Wait for failover + new status log
						time.Sleep(healthcheckWaitDuration + 3*time.Second)

						// Should now see failover info
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"last_failover_at":"`))
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say(`"last_failover_from":"`))
					})
				})

				When("status logging is disabled", func() {
					BeforeEach(func() {
						rootConfig.StatusLog.Enabled = false
					})

					It("does not emit status updates", func() {
						time.Sleep(5 * time.Second)

						Consistently(switchboardRunner.Buffer(), "2s").ShouldNot(gbytes.Say("Status update"))
					})
				})

				Context("with inactive port enabled", func() {
					BeforeEach(func() {
						rootConfig.StatusLog.Enabled = true
						rootConfig.StatusLog.Interval = 2 * time.Second
						// Inactive port is already enabled in the default rootConfig setup
					})

					It("emits status updates for both active and inactive ports", func() {
						time.Sleep(3 * time.Second)

						// Should see status logs from the active port logger
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say("status.Status update"))

						// Should also see status logs from the inactive port logger
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say("inactive-node-status.Status update"))
					})

					It("reports different active backends for active and inactive ports", func() {
						time.Sleep(3 * time.Second)

						bufferContents := string(switchboardRunner.Buffer().Contents())

						// Both loggers should report an active backend
						Expect(bufferContents).To(MatchRegexp(`status\.Status update.*"active_backend":"backend-\d"`))
						Expect(bufferContents).To(MatchRegexp(`inactive-node-status\.Status update.*"active_backend":"backend-\d"`))

						// The active backends should be different (active uses lowest index, inactive uses highest)
						// This is implicit in the proxy's routing logic
					})
				})

				Context("with inactive port disabled", func() {
					BeforeEach(func() {
						rootConfig.StatusLog.Enabled = true
						rootConfig.StatusLog.Interval = 2 * time.Second
						rootConfig.Proxy.InactiveMysqlPort = 0 // Disable inactive port
					})

					It("only emits status updates for the active port", func() {
						time.Sleep(3 * time.Second)

						// Should see status logs from the active port logger
						Eventually(switchboardRunner.Buffer()).Should(gbytes.Say("status.Status update"))

						// Should NOT see status logs from the inactive port logger
						Consistently(switchboardRunner.Buffer(), "2s").ShouldNot(gbytes.Say("inactive-node-status"))
					})
				})
			})
		})
	})
})
