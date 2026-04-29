package node_manager_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"code.cloudfoundry.org/lager/v3/lagertest"

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
			MonitClient:   fakeMonit,
			StateFilePath: filepath.Join(tempDir, "state.txt"),
			Logger:        lagertest.NewTestLogger("node_manager"),
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

		When("the process client fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceBootstrap(nil)
				Expect(err).To(MatchError(`start error`))
			})
		})

		When("the process client starts successfully", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(nil)
			})

			It("returns success", func() {
				msg, err := mgr.StartServiceBootstrap(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("NEEDS_BOOTSTRAP")))
				Expect(msg).To(Equal(`cluster bootstrap successful`))
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

		When("the process client fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceJoin(nil)
				Expect(err).To(MatchError(`start error`))
			})
		})

		When("joining an existing cluster and start succeeds", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(nil)
			})

			It("returns success", func() {
				msg, err := mgr.StartServiceJoin(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("CLUSTERED")))
				Expect(msg).To(Equal(`join cluster successful`))
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

		When("the process client fails to start a service", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(fmt.Errorf(`start error`))
			})

			It("returns an error", func() {
				_, err := mgr.StartServiceSingleNode(nil)
				Expect(err).To(MatchError(`start error`))
			})
		})

		When("the process client starts successfully", func() {
			BeforeEach(func() {
				fakeMonit.StartReturns(nil)
			})

			It("returns success", func() {
				msg, err := mgr.StartServiceSingleNode(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(os.ReadFile(mgr.StateFilePath)).To(Equal([]byte("SINGLE_NODE")))
				Expect(msg).To(Equal(`single node start successful`))
			})
		})
	})

	Context("StopService", func() {
		When("the process client fails to stop a service", func() {
			BeforeEach(func() {
				fakeMonit.StopReturns(fmt.Errorf(`stop error`))
			})

			It("returns an error", func() {
				_, err := mgr.StopService(nil)
				Expect(err).To(MatchError(`stop error`))
			})
		})

		When("stop succeeds", func() {
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
		When("the process client fails", func() {
			BeforeEach(func() {
				fakeMonit.StatusReturns("", fmt.Errorf(`status error`))
			})

			It("returns an error", func() {
				_, err := mgr.GetStatus(nil)
				Expect(err).To(MatchError(`status error`))
			})
		})

		When("a status is returned", func() {
			BeforeEach(func() {
				fakeMonit.StatusReturns("running", nil)
			})

			It("returns the same status", func() {
				status, err := mgr.GetStatus(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(`running`))
			})
		})
	})
})
