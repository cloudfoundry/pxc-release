package statuslogger_test

import (
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
	"github.com/cloudfoundry-incubator/switchboard/runner/statuslogger"
)

var _ = Describe("StatusLogger", func() {
	var (
		logger          lager.Logger
		backends        []*domain.Backend
		clusterMonitor  *monitor.ClusterMonitor
		statusLogRunner *statuslogger.StatusLogger
		statusProcess   ifrit.Process
		logInterval     time.Duration
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("StatusLogger test")
		logInterval = 100 * time.Millisecond

		backendConfigs := []config.Backend{
			{
				Name:           "backend-0",
				Host:           "127.0.0.1",
				Port:           3306,
				StatusPort:     8080,
				StatusEndpoint: "api/v1/status",
			},
			{
				Name:           "backend-1",
				Host:           "127.0.0.2",
				Port:           3306,
				StatusPort:     8080,
				StatusEndpoint: "api/v1/status",
			},
		}

		backends = domain.NewBackends(backendConfigs, logger)

		// Create a cluster monitor (we won't actually start it for these tests)
		clusterMonitor = monitor.NewClusterMonitor(
			nil, // client
			false,
			backends,
			1*time.Second,
			logger.Session("cluster-monitor"),
			true,
		)

		statusLogRunner = statuslogger.NewStatusLogger(
			backends,
			clusterMonitor,
			logInterval,
			logger.Session("status-logger"),
		)
	})

	AfterEach(func() {
		if statusProcess != nil {
			statusProcess.Signal(os.Kill)
			Eventually(statusProcess.Wait()).Should(Receive())
		}
	})

	Describe("Run", func() {
		It("starts up and logs status messages", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))
			Eventually(logger).Should(gbytes.Say("backend_count"))
			Eventually(logger).Should(gbytes.Say("2")) // We have 2 backends in the test
		})

		It("shuts down gracefully when signalled", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			statusProcess.Signal(os.Interrupt)
			Eventually(statusProcess.Wait()).Should(Receive(BeNil()))

			Eventually(logger).Should(gbytes.Say("Status logger received shutdown signal"))
		})

		It("logs periodic status updates", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Wait for at least one status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))
		})

		It("includes healthy/total backend counts in status logs", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Wait for a status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))

			// Verify backend counts are present
			Eventually(logger).Should(gbytes.Say("healthy_backends"))
			Eventually(logger).Should(gbytes.Say("total_backends"))
		})

		It("includes connection count in status logs", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Wait for a status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))

			// Verify total_connections field is present
			Eventually(logger).Should(gbytes.Say("total_connections"))
		})

		It("includes active backend information in status logs", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Wait for a status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))

			// Verify active_backend field is present
			Eventually(logger).Should(gbytes.Say("active_backend"))
		})

		It("has a buffered channel to prevent monitor blocking during shutdown", func() {
			// The channel has capacity 1 to prevent the monitor from blocking
			// if it tries to send during the brief shutdown window.
			// This test verifies the channel is buffered.
			testLogger := statuslogger.NewStatusLogger(
				backends,
				clusterMonitor,
				logInterval,
				logger.Session("status-logger-buffer"),
			)

			// Verify channel has buffer capacity
			ch := testLogger.ActiveBackendChan()
			Expect(cap(ch)).To(Equal(1), "Channel should have capacity 1 to prevent blocking")
		})

		It("ensures backend listener goroutine exits before Run returns", func() {
			testLogger := statuslogger.NewStatusLogger(
				backends,
				clusterMonitor,
				logInterval,
				logger.Session("status-logger-cleanup"),
			)

			statusProcess = ifrit.Invoke(testLogger)
			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Signal shutdown and wait
			statusProcess.Signal(os.Interrupt)
			Eventually(statusProcess.Wait()).Should(Receive(BeNil()),
				`Expected backend listener to eventually stop cleanly, but it did not`)

			// Verify goroutine stopped - channel should block since no one is reading
			// Create a local copy to avoid race with other tests
			backend0 := backends[0]
			backend1 := backends[1]
			ch := testLogger.ActiveBackendChan()

			sendCompleted := make(chan bool)
			go func() {
				defer GinkgoRecover() // Recover from potential panic if channel closes
				ch <- backend0        // Fills buffer
				ch <- backend1        // Blocks forever if goroutine stopped
				sendCompleted <- true
			}()

			Consistently(sendCompleted, 200*time.Millisecond).ShouldNot(Receive())
		})
	})

	Describe("Status logging with active backend", func() {
		It("logs the active backend information when one is set", func() {
			// Start the monitor so it initializes the health map
			monitorStopChan := make(chan interface{})
			clusterMonitor.Monitor(monitorStopChan)
			defer close(monitorStopChan)

			// Give the monitor a moment to initialize
			time.Sleep(10 * time.Millisecond)

			// Register the status logger to receive active backend updates
			clusterMonitor.RegisterBackendSubscriber(statusLogRunner.ActiveBackendChan())

			// Start the status logger
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Simulate setting an active backend by making one healthy
			backends[0].SetHealthy()

			// Wait for a status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))

			// The active backend should be "none" initially since we're not actually
			// running the monitor's healthcheck loop with a real HTTP client
			Eventually(logger).Should(gbytes.Say("active_backend"))
		})

		It("updates active backend when notified via channel", func() {
			// Initialize a fresh status logger
			testLogger := statuslogger.NewStatusLogger(
				backends,
				clusterMonitor,
				logInterval,
				logger.Session("status-logger-2"),
			)

			statusProcess = ifrit.Invoke(testLogger)
			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Send an active backend update through the channel
			go func() {
				time.Sleep(50 * time.Millisecond)
				testLogger.ActiveBackendChan() <- backends[0]
			}()

			// Wait for a status update that should include the active backend name
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))
			Eventually(logger).Should(gbytes.Say("backend-0"))
		})
	})

	Describe("Failover tracking", func() {
		It("logs failover information when backend changes", func() {
			// Initialize a fresh status logger
			testLogger := statuslogger.NewStatusLogger(
				backends,
				clusterMonitor,
				logInterval,
				logger.Session("status-logger-failover"),
			)

			statusProcess = ifrit.Invoke(testLogger)
			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Set initial backend
			testLogger.ActiveBackendChan() <- backends[0]
			time.Sleep(50 * time.Millisecond)

			// Trigger a failover by changing to a different backend
			testLogger.ActiveBackendChan() <- backends[1]
			time.Sleep(50 * time.Millisecond)

			// Wait for status update that should include failover info
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))
			Eventually(logger).Should(gbytes.Say("last_failover_at"))
			Eventually(logger).Should(gbytes.Say("last_failover_from"))
			Eventually(logger).Should(gbytes.Say("backend-0"))
			Eventually(logger).Should(gbytes.Say(`"total_failovers":1`))
		})

		It("does not log failover info when no failover has occurred", func() {
			statusProcess = ifrit.Invoke(statusLogRunner)

			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Wait for status update
			Eventually(logger, 2*logInterval).Should(gbytes.Say("Status update"))

			// Should NOT include failover fields when no backend has been set
			Consistently(logger, 100*time.Millisecond).ShouldNot(gbytes.Say("last_failover_at"))
		})

		It("continues to log failover info indefinitely after it occurs", func() {
			// Initialize a fresh status logger
			testLogger := statuslogger.NewStatusLogger(
				backends,
				clusterMonitor,
				logInterval,
				logger.Session("status-logger-persistent"),
			)

			statusProcess = ifrit.Invoke(testLogger)
			Eventually(logger).Should(gbytes.Say("Status logger starting"))

			// Set initial backend
			testLogger.ActiveBackendChan() <- backends[0]
			time.Sleep(50 * time.Millisecond)

			// Trigger a failover
			testLogger.ActiveBackendChan() <- backends[1]
			time.Sleep(50 * time.Millisecond)

			// First status update should include failover
			Eventually(logger, 2*logInterval).Should(gbytes.Say("last_failover_at"))

			// Wait for another status update (well past any time window)
			time.Sleep(logInterval + 50*time.Millisecond)

			// Should STILL include failover info (no time limit)
			Eventually(logger).Should(gbytes.Say("last_failover_at"))
			Eventually(logger).Should(gbytes.Say("backend-0"))
		})
	})
})
