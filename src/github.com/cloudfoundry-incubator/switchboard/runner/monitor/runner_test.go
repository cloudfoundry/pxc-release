package monitor_test

import (
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor/monitorfakes"
)

var _ = Describe("Monitor Runner", func() {
	It("shuts down gracefully when signalled", func() {
		m := new(monitorfakes.FakeMonitor)

		logger := slog.New(slog.NewJSONHandler(GinkgoWriter, nil))
		monitorRunner := monitor.NewRunner(m, logger)
		monitorProcess := ifrit.Invoke(monitorRunner)

		shutdownChan := m.MonitorArgsForCall(0)
		Consistently(shutdownChan).ShouldNot(BeClosed())

		monitorProcess.Signal(os.Kill)
		Eventually(monitorProcess.Wait()).Should(Receive())

		Expect(shutdownChan).To(BeClosed())
	})
})
