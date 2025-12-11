package config_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strconv"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
				Socket:   "/var/vcap/sys/run/pxc-mysql/mysqld.sock",
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

				address := net.JoinHostPort(rootConfig.Host, strconv.Itoa(rootConfig.Port))
				conn, err := net.Dial("tcp", address)
				Expect(err).NotTo(HaveOccurred())

				msg, err := io.ReadAll(conn)
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
			rootConfig.Host = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Host"))
		})

		It("returns an error if Port is blank", func() {
			rootConfig.Port = 0
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Port"))
		})

		It("returns an error if AvailableWhenReadOnly is blank", func() {
			rootConfig.AvailableWhenReadOnly = false
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error if AvailableWhenDonor is blank", func() {
			rootConfig.AvailableWhenDonor = false
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error if DB.Socket is blank", func() {
			rootConfig.DB.Socket = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Socket"))
		})

		It("returns an error if DB.User is blank", func() {
			rootConfig.DB.User = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("User"))
		})

		It("returns an error if DB.Password is blank", func() {
			rootConfig.DB.Password = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Password"))
		})

		It("returns an error if Monit.Host is blank", func() {
			rootConfig.Monit.Host = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Host"))
		})

		It("returns an error if Monit.User is blank", func() {
			rootConfig.Monit.User = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("User"))
		})

		It("returns an error if Monit.Port is blank", func() {
			rootConfig.Monit.Port = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Port"))
		})

		It("returns an error if Monit.Password is blank", func() {
			rootConfig.Monit.Password = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Password"))
		})

		It("returns an error if MysqldPath is blank", func() {
			rootConfig.MysqldPath = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MysqldPath"))
		})

		It("returns an error if Monit.ServiceName is blank", func() {
			rootConfig.Monit.ServiceName = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ServiceName"))
		})

		It("returns an error if SidecarEndpoint.Username is blank", func() {
			rootConfig.SidecarEndpoint.Username = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Username"))
		})

		It("returns an error if SidecarEndpoint.Password is blank", func() {
			rootConfig.SidecarEndpoint.Password = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Password"))
		})

		It("returns a valid logger", func() {
			Expect(rootConfig.Logger).ToNot(BeNil())
		})
	})
})
