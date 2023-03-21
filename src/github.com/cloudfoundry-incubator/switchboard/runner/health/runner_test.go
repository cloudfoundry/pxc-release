package health_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/switchboard/runner/health"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("HealthRunner", func() {

	var (
		healthPort    int
		healthRunner  ifrit.Runner
		healthProcess ifrit.Process
	)

	BeforeEach(func() {

		healthPort = 10000 + GinkgoParallelProcess()

		healthRunner = health.NewRunner("127.0.0.1:" + strconv.Itoa(healthPort))
		healthProcess = ifrit.Invoke(healthRunner)
		isReady := healthProcess.Ready()
		Eventually(isReady, "30s").Should(BeClosed(), "Error starting Health Runner")
	})

	AfterEach(func() {
		healthProcess.Signal(os.Kill)
		err := <-healthProcess.Wait()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when the runner is running", func() {
		It("accepts connections on health port", func() {
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", healthPort))
			Expect(err).ToNot(HaveOccurred())

			err = conn.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("accepts HTTP GET connections on / on the health port", func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d", healthPort), nil)
			Expect(err).NotTo(HaveOccurred())

			res, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.StatusCode).To(Equal(200))
		})
	})

	It("shuts down gracefully when signalled", func() {
		healthProcess.Signal(os.Kill)
		Eventually(healthProcess.Wait()).Should(Receive())

		_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", healthPort))
		Expect(err).To(HaveOccurred())
	})
})
