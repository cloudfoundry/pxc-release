package start_manager_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/fakes"
)

var _ = Describe("StartManagerRunner", func() {

	var (
		fakeManager *fakes.FakeStartManager
		successCmd  *exec.Cmd
		runner      start_manager.Runner
	)
	cmdDurationInSec := 0.3
	readyTimeout := 0.1

	BeforeEach(func() {
		testLogger := lagertest.NewTestLogger("start_manager")
		successCmd = exec.Command("sleep", fmt.Sprintf("%f", cmdDurationInSec))

		fakeManager = &fakes.FakeStartManager{}

		runner = start_manager.NewRunner(fakeManager, testLogger)
	})

	Context("When StartManager.Execute succeeds", func() {

		BeforeEach(func() {
			fakeManager.GetMysqlCmdReturns(successCmd, nil)
			fakeManager.ExecuteStub = func() error {
				err := successCmd.Start()
				Expect(err).ToNot(HaveOccurred())
				return nil
			}
		})

		It("Closes the ready channel and waits for mysql to exit", func() {
			signals := make(chan os.Signal)
			ready := make(chan struct{})

			runErr := make(chan error)
			go func() {
				runErr <- runner.Run(signals, ready)
			}()

			Eventually(ready, readyTimeout).Should(BeClosed())
			Eventually(runErr, cmdDurationInSec+readyTimeout).Should(Receive(nil))
		})
	})

	Context("When StartManager.Execute fails", func() {
		errorMsg := "exec error"
		BeforeEach(func() {
			fakeManager.ExecuteReturns(errors.New(errorMsg))
		})

		It("Returns the error without closing ready", func() {
			signals := make(chan os.Signal)
			ready := make(chan struct{})

			err := runner.Run(signals, ready)
			Expect(err).To(MatchError(errorMsg))

			Expect(ready).ToNot(BeClosed())
		})
	})
})
