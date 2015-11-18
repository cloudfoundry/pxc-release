package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/test_helpers"
	"github.com/fraenkel/candiedyaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Bootstrap", func() {

	var (
		testServers     []*httptest.Server
		endpointHandler *test_helpers.EndpointHandler
		rootConfig      *config.Config
		cmd             *exec.Cmd
	)

	BeforeEach(func() {
		endpointHandler = test_helpers.NewEndpointHandler()
		endpointHandler.StubEndpoint("/stop_mysql")
	})

	JustBeforeEach(func() {
		rootConfig = &config.Config{}
		testServers = []*httptest.Server{}
		for i := 0; i < 3; i++ {
			newServer := httptest.NewServer(endpointHandler)
			testServers = append(testServers, newServer)

			rootConfig.HealthcheckURLs = append(rootConfig.HealthcheckURLs, newServer.URL)
		}

		configPath := writeConfig(rootConfig)
		cmd = exec.Command(binaryPath, fmt.Sprintf("-configPath=%s", configPath))
	})

	AfterEach(func() {
		for _, ts := range testServers {
			ts.Close()
		}
	})

	// make api call to stop monitoring mysql process and we should get 200 ok
	Context("when we get 200 response for shutting down mariadb_ctrl from mysql node", func() {

		It("sends request to /stop_mysql on each node", func() {
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))
			Expect(endpointHandler.Handlers["/stop_mysql"].ServeHTTPCallCount()).To(Equal(3))
		})
	})

	Context("when we get non-200 response for shutting down mariadb_ctrl from mysql node", func() {
		BeforeEach(func() {
			fakeHandler := &fakes.FakeHandler{}
			fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "fake-error")
			}
			endpointHandler.StubEndpoint("/stop_mysql", fakeHandler)
		})

		It("sends request to /stop_mysql on each node", func() {
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).ToNot(BeZero())
			Expect(session.Err).To(gbytes.Say("Non 200 response"))

			Expect(endpointHandler.Handlers["/stop_mysql"].ServeHTTPCallCount()).To(Equal(1))
		})
	})
})

func writeConfig(rootConfig *config.Config) string {
	configPath, err := ioutil.TempFile(tempDir, "config.yml")
	Expect(err).ToNot(HaveOccurred())

	fileToWrite, err := os.Create(configPath.Name())
	Expect(err).ToNot(HaveOccurred())

	encoder := candiedyaml.NewEncoder(fileToWrite)
	err = encoder.Encode(rootConfig)
	Expect(err).ToNot(HaveOccurred())

	return configPath.Name()
}
