package node_runner_test

import (
	"errors"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry/mariadb_ctrl/start_manager/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_runner"
)

var _ = Describe("StartManagerRunner", func() {

	var (
		fakeManager    *fakes.FakeStartManager
		longRunningCmd *exec.Cmd
		runner         node_runner.Runner
	)

	BeforeEach(func() {
		testLogger := lagertest.NewTestLogger("node_runner")
		longRunningCmd = exec.Command("yes")

		fakeManager = &fakes.FakeStartManager{}

		runner = node_runner.NewRunner(fakeManager, testLogger)
	})

	AfterEach(func() {
		if longRunningCmd.Process != nil {
			_ = longRunningCmd.Process.Signal(os.Kill) //ignore error
		}
	})

	Context("When StartManager.Execute succeeds", func() {

		BeforeEach(func() {
			fakeManager.GetMysqlCmdReturns(longRunningCmd, nil)
			fakeManager.ExecuteStub = func() error {
				err := longRunningCmd.Start()
				Expect(err).ToNot(HaveOccurred())
				return nil
			}
		})

		It("Closes the ready channel and waits for mysqld to exit", func() {
			signals := make(chan os.Signal)
			ready := make(chan struct{})

			runErr := make(chan error)
			go func() {
				runErr <- runner.Run(signals, ready)
			}()

			Eventually(ready).Should(BeClosed())

			Consistently(runErr).ShouldNot(Receive())
			longRunningCmd.Process.Signal(os.Kill)
			Eventually(runErr).Should(Receive(nil))
		})

		Context("And the runner is signaled", func() {

			It("Tells the mysqld process to shutdown", func() {
				signals := make(chan os.Signal)
				ready := make(chan struct{})

				runErr := make(chan error)
				go func() {
					runErr <- runner.Run(signals, ready)
				}()

				Eventually(ready).Should(BeClosed())

				Consistently(runErr).ShouldNot(Receive())
				signals <- os.Kill
				Eventually(runErr).Should(Receive())
				Expect(fakeManager.ShutdownCallCount()).To(Equal(1))
			})
		})
	})

	Context("When StartManager.Execute fails", func() {
		const errorMsg = "exec error"
		var signals chan os.Signal
		var ready chan struct{}

		BeforeEach(func() {
			fakeManager.ExecuteReturns(errors.New(errorMsg))

			signals = make(chan os.Signal)
			ready = make(chan struct{})
		})

		It("Returns the error", func() {
			err := runner.Run(signals, ready)
			Expect(err).To(MatchError(errorMsg))
		})

		It("does not close the ready channel", func() {
			err := runner.Run(signals, ready)
			Expect(err).To(HaveOccurred())

			Consistently(ready).ShouldNot(BeClosed())
		})

		It("Tells the mysqld process to shutdown", func() {
			runErr := make(chan error)
			go func() {
				runErr <- runner.Run(signals, ready)
			}()

			Eventually(runErr).Should(Receive())
			Expect(fakeManager.ShutdownCallCount()).To(Equal(1))
		})
	})
})
