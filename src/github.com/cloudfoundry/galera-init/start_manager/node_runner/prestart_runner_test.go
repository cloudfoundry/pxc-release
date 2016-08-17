package node_runner_test

import (
	"errors"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_runner"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/start_managerfakes"
)

var _ = Describe("StartManagerRunner", func() {

	var (
		fakeManager    *start_managerfakes.FakeStartManager
		longRunningCmd *exec.Cmd
		runner         node_runner.PrestartRunner
	)

	BeforeEach(func() {
		testLogger := lagertest.NewTestLogger("node_runner")
		longRunningCmd = exec.Command("yes")

		fakeManager = &start_managerfakes.FakeStartManager{}

		runner = node_runner.NewPrestartRunner(fakeManager, testLogger)
	})

	AfterEach(func() {
		if longRunningCmd.Process != nil {
			_ = longRunningCmd.Process.Signal(os.Kill) //ignore error
		}
	})

	Context("When StartManager.Execute succeeds", func() {

		BeforeEach(func() {
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
			//fmt.Println("Sending os.kill to long running process...")
			signals <- os.Kill
			Eventually(runErr).Should(Receive(nil))
		})

		Context("And the runner is signaled", func() {

			It("Does not tell the mysqld process to shutdown", func() {
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
				Expect(fakeManager.ShutdownCallCount()).To(Equal(0))
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

	Context("When StartManager.Execute succeeds for prestart on bootstrap node", func() {

		BeforeEach(func() {
			fakeManager.GetMysqlCmdReturns(nil, nil)
			fakeManager.ExecuteStub = func() error {
				return nil
			}
		})

		It("Closes the ready channel and exits", func() {
			signals := make(chan os.Signal)
			ready := make(chan struct{})

			runErr := make(chan error)
			go func() {
				runErr <- runner.Run(signals, ready)
			}()

			Eventually(ready).Should(BeClosed())
			Consistently(runErr).ShouldNot(Receive())
			signals <- os.Kill
			Eventually(runErr).Should(Receive(nil))
		})

	})
})
