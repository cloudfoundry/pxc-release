package bridge_test

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/runner/bridge"
)

var _ = Describe("Bridge Runner", func() {
	It("does not leak a goroutine per traffic or backend channel event", func() {
		timeout := 100 * time.Millisecond
		proxyPort := 10100 + GinkgoParallelProcess()
		logger := lagertest.NewTestLogger("ProxyRunner goroutine-leak test")

		proxyRunner := bridge.NewRunner("127.0.0.1:"+strconv.Itoa(proxyPort), timeout, logger)
		proxyProcess := ifrit.Invoke(proxyRunner)

		Eventually(func() error {
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
			if err == nil {
				_ = conn.Close()
			}
			return err
		}).ShouldNot(HaveOccurred())

		// Settle before measuring baseline.
		time.Sleep(10 * time.Millisecond)
		before := runtime.NumGoroutine()

		const numEvents = 30
		for i := 0; i < numEvents; i++ {
			proxyRunner.TrafficEnabledChan <- i%2 == 0
		}

		time.Sleep(10 * time.Millisecond)
		after := runtime.NumGoroutine()

		// With the bug each event leaks one goroutine; allow a small margin for noise.
		Expect(after).To(BeNumerically("<=", before+5),
			"goroutine count should not grow with repeated TrafficEnabledChan events")

		proxyProcess.Signal(os.Kill)
		Eventually(proxyProcess.Wait()).Should(Receive())
	})

	It("does not leak goroutines on shutdown", func() {
		timeout := 100 * time.Millisecond
		proxyPort := 10200 + GinkgoParallelProcess()
		logger := lagertest.NewTestLogger("shutdown-leak test")

		// Let goroutines from the prior spec settle before measuring baseline.
		time.Sleep(10 * time.Millisecond)
		before := runtime.NumGoroutine()

		proxyRunner := bridge.NewRunner("127.0.0.1:"+strconv.Itoa(proxyPort), timeout, logger)
		proxyProcess := ifrit.Invoke(proxyRunner)

		Eventually(func() error {
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
			if err == nil {
				_ = conn.Close()
			}
			return err
		}).ShouldNot(HaveOccurred())

		proxyProcess.Signal(os.Kill)
		Eventually(proxyProcess.Wait()).Should(Receive())

		// A permanently-leaked goroutine (stuck on unbuffered channel send) will
		// never exit, so Eventually times out and the test fails. Transient runtime
		// goroutines settle within the window and are not false positives.
		Eventually(func() int { return runtime.NumGoroutine() }, "500ms", "10ms").Should(
			BeNumerically("<=", before),
		)
	})

	It("logs a non-nil error when there is no active backend", func() {
		timeout := 100 * time.Millisecond
		proxyPort := 10300 + GinkgoParallelProcess()
		logger := lagertest.NewTestLogger("no-backend-error-log test")

		proxyRunner := bridge.NewRunner("127.0.0.1:"+strconv.Itoa(proxyPort), timeout, logger)
		proxyProcess := ifrit.Invoke(proxyRunner)
		defer func() {
			proxyProcess.Signal(os.Kill)
			Eventually(proxyProcess.Wait()).Should(Receive())
		}()

		// Dial with no active backend set — triggers the nil-error log path.
		Eventually(func() error {
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
			if err == nil {
				_ = conn.Close()
			}
			return err
		}).ShouldNot(HaveOccurred())

		Eventually(func() bool {
			for _, entry := range logger.Logs() {
				if strings.Contains(entry.Message, "No active backend") {
					return true
				}
			}
			return false
		}).Should(BeTrue(), "expected 'No active backend' log entry")
	})

	It("shuts down gracefully when signalled", func() {
		timeout := 100 * time.Millisecond

		proxyPort := 10000 + GinkgoParallelProcess()
		logger := lagertest.NewTestLogger("ProxyRunner test")

		proxyRunner := bridge.NewRunner("127.0.0.1:"+strconv.Itoa(proxyPort), timeout, logger)
		proxyProcess := ifrit.Invoke(proxyRunner)

		Eventually(func() error {
			_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
			return err
		}).ShouldNot(HaveOccurred())

		proxyProcess.Signal(os.Kill)

		smallEpsilon := 10 * time.Millisecond

		Consistently(func() error {
			_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
			return err
		},
			timeout-smallEpsilon,
		).Should(Succeed())

		Eventually(proxyProcess.Wait()).Should(Receive())

		_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
		Expect(err).To(HaveOccurred())
	})
})
