package bridge_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/runner/bridge"
)

var _ = Describe("Bridge Runner", func() {
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
