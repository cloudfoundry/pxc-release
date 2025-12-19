package main_test

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/test_helpers"
)

var _ = Describe("Galera Agent", func() {
	var (
		session         *gexec.Session
		serverAuthority *certtest.Authority
		galeraAgentPort int
	)

	BeforeEach(func() {
		var err error
		serverAuthority, err = certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := serverAuthority.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())

		serverCertPEM, serverKeyPEM, err := serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		galeraAgentPort = test_helpers.RandomPort()
		cfg := config.Config{
			DB: config.DBConfig{
				Password: "root-password",
			},
			Monit: config.MonitConfig{
				Host:                          "foo",
				User:                          "foo",
				Port:                          "foo",
				Password:                      "foo",
				MysqlStateFilePath:            "foo",
				ServiceName:                   "foo",
				GaleraInitStatusServerAddress: "foo",
			},
			Host:       "localhost",
			Port:       galeraAgentPort,
			MysqldPath: "mysqld",
			MyCnfPath:  "my.cnf",
			SidecarEndpoint: config.SidecarEndpointConfig{
				Username: "basic-auth-username",
				Password: "basic-auth-password",
				TLS: config.EndpointTLS{
					Enabled:     true,
					Certificate: string(serverCertPEM),
					PrivateKey:  string(serverKeyPEM),
				},
			},
		}
		b, err := yaml.Marshal(&cfg)
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command(
			binaryPath,
			fmt.Sprintf("-config=%s", string(b)),
		)

		session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		session.Terminate()
	})

	It("Only accepts connections over TLS", func() {
		galeraAgentURL := fmt.Sprintf("http://127.0.0.1:%d/health", galeraAgentPort)
		Eventually(func() error {
			res, err := http.Get(galeraAgentURL)
			if err != nil {
				return err
			}

			if res.StatusCode == http.StatusOK {
				return nil
			}

			body, _ := io.ReadAll(res.Body)
			trimmedBody := strings.TrimSpace(string(body))
			return fmt.Errorf("received status code: %d, with body: %s", res.StatusCode, trimmedBody)
		}, "10s", "1s").Should(MatchError(`received status code: 400, with body: Client sent an HTTP request to an HTTPS server.`))
	})

	It("Accepts TLS connections", func() {
		serverCertPool, err := serverAuthority.CertPool()
		Expect(err).ToNot(HaveOccurred())

		tlsClientConfig, err := tlsconfig.Build().Client(
			tlsconfig.WithAuthority(serverCertPool),
		)

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
		}

		galeraAgentURL := fmt.Sprintf("https://127.0.0.1:%d/health", galeraAgentPort)

		Eventually(func() error {
			res, err := httpClient.Get(galeraAgentURL)
			if err != nil {
				return err
			}

			if res.StatusCode == http.StatusOK {
				return nil
			}

			body, _ := io.ReadAll(res.Body)
			trimmedBody := strings.TrimSpace(string(body))
			return fmt.Errorf("received status code: %d, with body: %s", res.StatusCode, trimmedBody)
		}, "10s", "1s").ShouldNot(HaveOccurred())
	})

})
