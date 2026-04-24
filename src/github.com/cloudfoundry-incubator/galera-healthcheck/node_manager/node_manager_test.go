package node_manager_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/onsi/gomega/ghttp"

	"github.com/cloudfoundry-incubator/galera-healthcheck/bpm_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/bpm_client/bpm_clientfakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager/node_managerfakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NodeManager", func() {
	var (
		mgr       *node_manager.NodeManager
		fakeMonit *node_managerfakes.FakeMonitClient
		tempDir   string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp(os.TempDir(), "tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
		})

		fakeMonit = &node_managerfakes.FakeMonitClient{}

		mgr = &node_manager.NodeManager{
			ServiceName:   "galera-init",
			ProcessClient: fakeMonit,
			StateFilePath: filepath.Join(tempDir, "state.txt"),
			Logger:        lagertest.NewTestLogger("monit_client"),
			Mutex:         &sync.Mutex{},
		}
	})

	Context("StartServiceBootstrap", func() {
		When("writing a state file fails", func() {
			BeforeEach(func() {
				mgr.StateFilePath = filepath.Join(tempDir, "invalid", "other")
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceBootstrap(nil)
				Expect(err).
					To(
						MatchError(
							fmt.Sprintf(`failed to initialize state file: open %s: no such file or directory`, mgr.StateFilePath),
						),
					)
			})
		})

		When("monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceBootstrap(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		When("monit starts successfully", func() {
			When("the service fails during initialization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceBootstrap(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			When("monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", fmt.Errorf("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceBootstrap(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			When("galera-init status endpoint returns a bad http status", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceBootstrap(nil)
					Expect(err).To(MatchError(`unexpected response from node: 500 Internal Server Error`))
				})
			})

			When("galera-init initializes successfully", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusOK, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns success", func() {
					msg, err := mgr.StartServiceBootstrap(nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("NEEDS_BOOTSTRAP")))
					Expect(msg).To(Equal(`cluster bootstrap successful`))
				})
			})
		})
	})

	Context("StartServiceJoin", func() {
		When("writing a state file fails", func() {
			BeforeEach(func() {
				mgr.StateFilePath = filepath.Join(tempDir, "invalid", "other")
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceJoin(nil)
				Expect(err).
					To(
						MatchError(
							fmt.Sprintf(`failed to initialize state file: open %s: no such file or directory`, mgr.StateFilePath),
						),
					)
			})
		})

		When("monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceJoin(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		When("joining an existing cluster", func() {
			When("the service fails during initialization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceJoin(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			When("monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", fmt.Errorf("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceJoin(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			When("galera-init status endpoint returns a bad http status", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceJoin(nil)
					Expect(err).To(MatchError(`unexpected response from node: 500 Internal Server Error`))
				})
			})

			When("galera-init initializes successfully", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusOK, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns success", func() {
					msg, err := mgr.StartServiceJoin(nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("CLUSTERED")))
					Expect(msg).To(Equal(`join cluster successful`))
				})
			})
		})
	})

	Context("StartServiceSingleNode", func() {
		When("writing a state file fails", func() {
			BeforeEach(func() {
				mgr.StateFilePath = filepath.Join(tempDir, "invalid", "other")
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceSingleNode(nil)
				Expect(err).
					To(
						MatchError(
							fmt.Sprintf(`failed to initialize state file: open %s: no such file or directory`, mgr.StateFilePath),
						),
					)
			})
		})

		When("monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceSingleNode(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		When("monit starts successfully", func() {
			When("the service fails during initialization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceSingleNode(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			When("monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", fmt.Errorf("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceSingleNode(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			When("galera-init status endpoint returns a bad http status", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceSingleNode(nil)
					Expect(err).To(MatchError(`unexpected response from node: 500 Internal Server Error`))
				})
			})

			When("galera-init initializes successfully", func() {
				var server *ghttp.Server

				BeforeEach(func() {
					server = ghttp.NewServer()

					server.RouteToHandler(
						"GET",
						"/",
						ghttp.RespondWith(http.StatusOK, nil),
					)

					mgr.GaleraInitAddress = server.Addr()

					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("running", nil)
				})

				AfterEach(func() {
					server.Close()
				})

				It("returns success", func() {
					msg, err := mgr.StartServiceSingleNode(nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("SINGLE_NODE")))
					Expect(msg).To(Equal(`single node start successful`))
				})
			})
		})
	})

	Context("StopService", func() {
		When("monit fails to stop a service", func() {
			BeforeEach(func() {
				fakeMonit.StopReturns(fmt.Errorf(`monit stop error`))
			})

			It("returns an error", func() {
				_, err := mgr.StopService(nil)
				Expect(err).To(MatchError(`monit stop error`))
			})
		})

		When("monit stops a service successfully", func() {
			It("returns success", func() {
				msg, err := mgr.StopService(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).To(Equal(`stop successful`))
			})

			It("does not modify the state file", func() {
				Expect(os.WriteFile(mgr.StateFilePath, []byte("PRE_EXISTING_CLUSTER_STATE"), 0o0644)).To(Succeed())

				_, err := mgr.StopService(nil)
				Expect(err).NotTo(HaveOccurred())

				contents, err := os.ReadFile(mgr.StateFilePath)
				Expect(err).NotTo(HaveOccurred())

				Expect(string(contents)).To(Equal("PRE_EXISTING_CLUSTER_STATE"))
			})
		})
	})

	Context("GetStatus", func() {
		When("monit fails", func() {
			BeforeEach(func() {
				fakeMonit.StatusReturns("", fmt.Errorf(`monit error`))
			})

			It("returns an error", func() {
				_, err := mgr.GetStatus(nil)
				Expect(err).To(MatchError(`monit error`))
			})
		})

		When("monit returns a status", func() {
			BeforeEach(func() {
				fakeMonit.StatusReturns("some monit status", nil)
			})

			It("returns the same status", func() {
				status, err := mgr.GetStatus(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(`some monit status`))
			})
		})
	})

	Describe("ProcessClient interface compatibility", func() {
		var (
			bpmClient     *bpm_client.BpmClient
			fakeRunner    *bpm_clientfakes.FakeCommandRunner
			mgr           *node_manager.NodeManager
			tempDir       string
		)

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp(os.TempDir(), "bpm-test")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.RemoveAll(tempDir)).To(Succeed())
			})

			fakeRunner = &bpm_clientfakes.FakeCommandRunner{}
			bpmClient = bpm_client.NewClient("/usr/bin/bpm", "test-job", "test-process", 30*time.Second, fakeRunner)

			mgr = &node_manager.NodeManager{
				ServiceName:   "galera-init",
				ProcessClient: bpmClient,
				StateFilePath: filepath.Join(tempDir, "state.txt"),
				Logger:        lagertest.NewTestLogger("test"),
				Mutex:         &sync.Mutex{},
			}
		})

		It("can use BpmClient as ProcessClient implementation", func() {
			fakeRunner.RunReturns([]byte("12345"), nil)

			status, err := mgr.GetStatus(&http.Request{})

			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("running"))
			Expect(fakeRunner.RunCallCount()).To(Equal(1))
		})

		It("can start service using BpmClient", func() {
			// First call is Start, second call is Status from waitForGaleraInit
			fakeRunner.RunReturnsOnCall(0, []byte(""), nil)  // bpm start
			fakeRunner.RunReturnsOnCall(1, []byte(""), nil)  // bpm pid returns empty (stopped)

			_, err := mgr.StartServiceBootstrap(&http.Request{})

			Expect(err).To(HaveOccurred()) // Will fail at waitForGaleraInit since service appears stopped
			Expect(fakeRunner.RunCallCount()).To(BeNumerically(">=", 2))
			
			command, args := fakeRunner.RunArgsForCall(0)
			Expect(command).To(Equal("/usr/bin/bpm"))
			Expect(args).To(Equal([]string{"start", "test-job", "-p", "test-process"}))

			// Verify state file was written
			stateContent, err := os.ReadFile(mgr.StateFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(stateContent)).To(Equal("NEEDS_BOOTSTRAP"))
		})
	})
})
