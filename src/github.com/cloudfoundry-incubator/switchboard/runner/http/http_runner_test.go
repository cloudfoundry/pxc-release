package http_test

import (
	"net"
	"net/http"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	httprunner "github.com/cloudfoundry-incubator/switchboard/runner/http"
	"github.com/cloudfoundry-incubator/switchboard/testing"
)

type stubHandler struct{}

func (f *stubHandler) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
}

var _ http.Handler = (*stubHandler)(nil)

var _ = Describe("HTTPRunner", func() {

	var (
		healthRunner  ifrit.Runner
		healthProcess ifrit.Process
		handler       *stubHandler
		runnerURL     string
	)

	BeforeEach(func() {
		handler = new(stubHandler)
		port := 10000 + GinkgoParallelNode()
		address := "127.0.0.1:" + strconv.Itoa(port)

		runnerURL = "http://" + address

		healthRunner = httprunner.NewHTTPRunner(address, handler)
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
		It("accepts HTTP GET connections on / on the health port", func() {
			req, err := http.NewRequest("GET", runnerURL, nil)
			Expect(err).NotTo(HaveOccurred())

			res, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.StatusCode).To(Equal(200))
		})
	})

	It("shuts down gracefully when signalled", func() {
		healthProcess.Signal(os.Kill)
		Eventually(healthProcess.Wait()).Should(Receive())

		_, err := net.Dial("tcp", runnerURL)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("HTTPRunnerWithTLS", func() {

	var (
		healthRunner  ifrit.Runner
		healthProcess ifrit.Process
		handler       *stubHandler
		runnerURL     string
		httpClient    *http.Client
	)

	BeforeEach(func() {
		caPEM, tlsCert, err := testing.GenerateSelfSignedCertificate("localhost")
		Expect(err).NotTo(HaveOccurred())

		serverTlsCfg, err := testing.ServerConfigFromCertificate(tlsCert)
		Expect(err).NotTo(HaveOccurred())

		clientTlsCfg, err := testing.ClientConfigFromAuthority(caPEM)
		Expect(err).NotTo(HaveOccurred())

		httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: clientTlsCfg}}

		handler = new(stubHandler)
		port := 10000 + GinkgoParallelNode()
		address := "127.0.0.1:" + strconv.Itoa(port)

		runnerURL = "https://" + address

		healthRunner = httprunner.NewHTTPRunnerWithTLS(address, handler, serverTlsCfg)
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
		It("accepts HTTP GET connections on / on the health port", func() {
			req, err := http.NewRequest("GET", runnerURL, nil)
			Expect(err).NotTo(HaveOccurred())

			res, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.StatusCode).To(Equal(200))
		})
	})

	It("shuts down gracefully when signalled", func() {
		healthProcess.Signal(os.Kill)
		Eventually(healthProcess.Wait()).Should(Receive())

		_, err := net.Dial("tcp", runnerURL)
		Expect(err).To(HaveOccurred())
	})
})
