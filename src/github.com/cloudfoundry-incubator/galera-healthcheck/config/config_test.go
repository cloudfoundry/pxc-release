package config_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	. "github.com/cloudfoundry-incubator/galera-healthcheck/config"
	. "github.com/cloudfoundry-incubator/galera-healthcheck/test_helpers"
)

func generatePrivateKeyPair(names ...string) (string, string) {
	authority, err := certtest.BuildCA("testCA")
	Expect(err).ToNot(HaveOccurred())

	certificate, err := authority.BuildSignedCertificate("localhost", certtest.WithDomains(names...))
	Expect(err).ToNot(HaveOccurred())

	caPEMBytes, privateKeyBytes, err := certificate.CertificatePEMAndPrivateKey()
	Expect(err).ToNot(HaveOccurred())

	return string(caPEMBytes), string(privateKeyBytes)

}

var _ = Describe("Config", func() {
	Describe("Validate", func() {

		var (
			rootConfig        *Config
			serverName        string
			healthCheckConfig Config
			ca                string
			privateKey        string
		)

		serverName = "PXC Release Testing Certificate"
		ca, privateKey = generatePrivateKeyPair(serverName)
		healthCheckConfig = Config{
			DB: DBConfig{
				User:     "vcap",
				Password: "password",
				Socket:   "",
			},
			Monit: MonitConfig{
				Host:                          "localhost",
				User:                          "vcap",
				Port:                          "2822",
				Password:                      "random-password",
				MysqlStateFilePath:            "/var/vcap/store/mysql/state.txt",
				ServiceName:                   "mariadb_ctrl",
				GaleraInitStatusServerAddress: "127.0.0.1:8114",
			},
			Host:                  "localhost",
			Port:                  8080,
			AvailableWhenDonor:    true,
			AvailableWhenReadOnly: false,
			MysqldPath:            "/var/vcap/packages/mariadb/bin/mysqld",
			MyCnfPath:             "/path/to/my.cnf",
			SidecarEndpoint: SidecarEndpointConfig{
				Username: "username",
				Password: "password",
				TLS: EndpointTLS{
					Enabled:     true,
					Certificate: ca,
					PrivateKey:  privateKey,
				},
			},
			Logger: nil,
		}

		BeforeEach(func() {
			var err error

			rawConfig, err := json.Marshal(healthCheckConfig)
			Expect(err).NotTo(HaveOccurred())

			osArgs := []string{
				"galera-healthcheck",
				fmt.Sprintf("-config=%s", rawConfig),
			}

			rootConfig, err = NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("can provide a TLS listener", func() {
			rootConfig.Port = RandomPort()
			l, err := rootConfig.NetworkListener()
			Expect(err).NotTo(HaveOccurred())
			defer l.Close()

			errCh := make(chan error)
			go func() {
				conn, err := l.Accept()
				if err != nil {
					errCh <- err
					return
				}
				defer conn.Close()
				conn.Write([]byte("foo"))
				errCh <- err
			}()

			block, _ := pem.Decode([]byte(rootConfig.SidecarEndpoint.TLS.Certificate))
			cert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())
			certPool := x509.NewCertPool()
			certPool.AddCert(cert)

			address := fmt.Sprintf("%s:%d", rootConfig.Host, rootConfig.Port)
			conn, err := tls.Dial("tcp", address, &tls.Config{
				RootCAs:    certPool,
				ServerName: serverName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(conn.Close()).To(Succeed())

			Eventually(errCh).Should(Receive(BeNil()))
		})

		When("tls is disabled", func() {
			It("provides a plaintext TCP listener", func() {
				rootConfig.Port = RandomPort()
				rootConfig.SidecarEndpoint.TLS.Enabled = false

				l, err := rootConfig.NetworkListener()
				Expect(err).NotTo(HaveOccurred())
				defer l.Close()

				errCh := make(chan error, 1)
				go func() {
					conn, err := l.Accept()
					defer conn.Close()
					_, _ = conn.Write([]byte("foo"))
					errCh <- err
				}()

				address := fmt.Sprintf("%s:%d", rootConfig.Host, rootConfig.Port)
				conn, err := net.Dial("tcp", address)
				Expect(err).NotTo(HaveOccurred())

				msg, err := ioutil.ReadAll(conn)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(msg)).To(Equal("foo"))
				Expect(conn.Close()).To(Succeed())
				Eventually(errCh).Should(Receive(BeNil()))
			})
		})

		When("tls is misconfigured", func() {
			It("returns an error", func() {
				rootConfig.SidecarEndpoint.TLS.Enabled = true
				rootConfig.SidecarEndpoint.TLS.Certificate = "not proper PEM content"

				_, err := rootConfig.NetworkListener()
				Expect(err).To(MatchError(`tls: failed to find any PEM data in certificate input`))
			})
		})

		It("returns an error if Host is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Host")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if AvailableWhenReadOnly is blank", func() {
			err := test_helpers.IsOptionalField(rootConfig, "AvailableWhenReadOnly")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if AvailableWhenDonor is blank", func() {
			err := test_helpers.IsOptionalField(rootConfig, "AvailableWhenDonor")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.Socket is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.Socket")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.User is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.User")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Monit.Host is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Monit.Host")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Monit.User is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Monit.User")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Monit.Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Monit.Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Monit.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Monit.Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if MysqldPath is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "MysqldPath")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Monit.ServiceName is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Monit.ServiceName")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if SidecarEndpoint.Username is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "SidecarEndpoint.Username")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if SidecarEndpoint.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "SidecarEndpoint.Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns a valid logger", func() {
			Expect(rootConfig.Logger).ToNot(BeNil())
		})
	})
})
