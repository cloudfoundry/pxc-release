package node_manager_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/pkg/errors"

	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager/node_managerfakes"
)

var _ = Describe("NodeManager", func() {
	var (
		mgr       *node_manager.NodeManager
		fakeMonit *node_managerfakes.FakeMonitClient
		tempDir   string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir(os.TempDir(), "tmp")
		Expect(err).NotTo(HaveOccurred())

		fakeMonit = &node_managerfakes.FakeMonitClient{}

		mgr = &node_manager.NodeManager{
			ServiceName:   "galera-init",
			MonitClient:   fakeMonit,
			StateFilePath: filepath.Join(tempDir, "state.txt"),
			Logger:        lagertest.NewTestLogger("monit_client"),
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Context("StartServiceBootstrap", func() {
		Context("when writing a state file fails", func() {
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

		Context("when monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(errors.New(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceBootstrap(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		Context("when monit starts successfully", func() {
			Context("when the service fails during initailization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceBootstrap(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			Context("when monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", errors.New("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceBootstrap(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			Context("when galera-init status endpoint returns a bad http status", func() {
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

			Context("when galera-init initializes successfully", func() {
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
					Expect(ioutil.ReadFile(mgr.StateFilePath)).To(Equal([]byte("NEEDS_BOOTSTRAP")))
					Expect(msg).To(Equal(`cluster bootstrap successful`))
				})
			})
		})
	})

	Context("StartServiceJoin", func() {
		Context("when writing a state file fails", func() {
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

		Context("when monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(errors.New(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceJoin(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		Context("when joining an existing cluter", func() {
			Context("when the service fails during initailization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceJoin(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			Context("when monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", errors.New("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceJoin(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			Context("when galera-init status endpoint returns a bad http status", func() {
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

			Context("when galera-init initializes successfully", func() {
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
					Expect(ioutil.ReadFile(mgr.StateFilePath)).To(Equal([]byte("CLUSTERED")))
					Expect(msg).To(Equal(`join cluster successful`))
				})
			})
		})
	})

	Context("StartServiceSingleNode", func() {
		Context("when writing a state file fails", func() {
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

		Context("when monit fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(errors.New(`monit start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceSingleNode(nil)
				Expect(err).To(MatchError(`monit start error`))
			})
		})

		Context("when monit starts successfully", func() {
			Context("when the service fails during initailization", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("failing", nil)
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceSingleNode(nil)
					Expect(err).To(MatchError(`job failed during startup`))
				})
			})

			Context("when monit becomes unavailable during startup", func() {
				BeforeEach(func() {
					fakeMonit.StartReturns(nil)
					fakeMonit.StatusReturns("", errors.New("monit communication error"))
				})

				It("returns an error", func() {
					_, err := mgr.StartServiceSingleNode(nil)
					Expect(err).To(MatchError(`error fetching status for service "galera-init"`))
				})
			})

			Context("when galera-init status endpoint returns a bad http status", func() {
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

			Context("when galera-init initializes successfully", func() {
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
					Expect(ioutil.ReadFile(mgr.StateFilePath)).To(Equal([]byte("SINGLE_NODE")))
					Expect(msg).To(Equal(`single node start successful`))
				})
			})
		})
	})

	Context("StopService", func() {
		Context("when monit fails to stop a service", func() {
			BeforeEach(func() {
				fakeMonit.StopReturns(errors.New(`monit stop error`))
			})

			It("returns an error", func() {
				_, err := mgr.StopService(nil)
				Expect(err).To(MatchError(`monit stop error`))
			})
		})

		Context("when monit stops a service successfully", func() {
			It("returns success", func() {
				msg, err := mgr.StopService(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).To(Equal(`stop successful`))
			})
		})
	})

	Context("GetStatus", func() {
		Context("when monit fails", func() {
			BeforeEach(func() {
				fakeMonit.StatusReturns("", errors.New(`monit error`))
			})

			It("returns an error", func() {
				_, err := mgr.GetStatus(nil)
				Expect(err).To(MatchError(`monit error`))
			})
		})

		Context("when monit returns a status", func() {
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
})
