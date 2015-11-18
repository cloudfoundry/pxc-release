package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/test_helpers"
	"github.com/fraenkel/candiedyaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Bootstrap", func() {

	const SERVER_COUNT = 3

	var (
		testServers      []*httptest.Server
		endpointHandlers []*test_helpers.EndpointHandler
		rootConfig       *config.Config
		cmd              *exec.Cmd
	)

	BeforeEach(func() {
		endpointHandlers = []*test_helpers.EndpointHandler{}
		for i := 0; i < SERVER_COUNT; i++ {
			endpointHandler := test_helpers.NewEndpointHandler()
			endpointHandler.StubEndpointWithStatus("/stop_mysql", http.StatusOK)
			endpointHandlers = append(endpointHandlers, endpointHandler)
		}
	})

	JustBeforeEach(func() {
		rootConfig = &config.Config{}
		testServers = []*httptest.Server{}
		for i := 0; i < SERVER_COUNT; i++ {
			newServer := httptest.NewServer(endpointHandlers[i])
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

	Context("when we get 200 response for shutting down mariadb_ctrl from mysql node", func() {

		It("sends request to /stop_mysql on each node", func() {
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))
			for _, handler := range endpointHandlers {
				Expect(handler.GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
			}
		})
	})

	Context("when we get non-200 response for shutting down mariadb_ctrl from mysql node", func() {
		BeforeEach(func() {
			endpointHandlers[0].StubEndpointWithStatus("/stop_mysql", http.StatusInternalServerError, "fake-error")
		})

		It("prints error to STDERR", func() {
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).ToNot(BeZero())
			Expect(session.Err).To(gbytes.Say("Non 200 response"))
			Expect(endpointHandlers[0].GetFakeHandler("/stop_mysql").ServeHTTPCallCount()).To(Equal(1))
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
