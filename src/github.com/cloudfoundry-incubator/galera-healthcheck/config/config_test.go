package config_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	. "github.com/cloudfoundry-incubator/galera-healthcheck/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
)

var _ = Describe("Config", func() {
	Describe("Validate", func() {

		var (
			rootConfig *Config
			rawConfig  string
		)

		BeforeEach(func() {
			rawConfig = `{
				"StatusEndpoint": "fake",
				"Host": "localhost",
				"Port": 8080,
				"ArbitratorNode": "false",
				"AvailableWhenReadOnly": false,
				"AvailableWhenDonor": true,
				"DB": {
					"User": "vcap",
					"Password": "password"
				},
				"Monit" : {
					"Host": "localhost",
					"User": "vcap",
					"Port": 2822,
					"Password": "random-password",
					"MysqlStateFilePath": "/var/vcap/store/mysql/state.txt",
					"ServiceName": "mariadb_ctrl",
					"GaleraInitStatusServerAddress": "127.0.0.1:8114"
				},
				"MysqldPath": "/var/vcap/packages/mariadb/bin/mysqld",
				"MyCnfPath": "/path/to/my.cnf",
				"SidecarEndpoint": {
					"Username": "username",
					"Password": "password",
					"TLS": {
						"Enabled": true,
						"Certificate": "-----BEGIN CERTIFICATE-----\nMIIEdDCCAlygAwIBAgIQcJMKm22mv8duqnxlET8tqjANBgkqhkiG9w0BAQsFADAh\nMR8wHQYDVQQDExZQWEMgUmVsZWFzZSBUZXN0aW5nIENBMB4XDTIyMDUxMDE0MTUx\nOFoXDTIzMTExMDE0MTIyMlowKjEoMCYGA1UEAxMfUFhDIFJlbGVhc2UgVGVzdGlu\nZyBDZXJ0aWZpY2F0ZTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAMpV\nOlpV8n7UzVZOzarEDC0pj48PqWsVROz7h+evdWvk8oI2VopAgh1h7lxfYRBVXBwF\nt8gUVFPcySGaaBFvNenZ0z07ht9K97ephws4zv+iFJZ9dXbXuO6ONz1/nErf4zlv\nij8PCNNJqyhkpZGfapl7yMt+tTioqaPmHYKvzg2vDOd2pIOlG6Db3LOMMkSkzaes\nqJBc8lAAAVFokItpeCsuibshW+fm0KGpZUd4Ja4x8pylbPJMnUebsCqsAv3fXHXz\nTX5j7Tk9NsvdfafUv7Ky55Cr+ZGECxwOyR7ikUJ/yqtVuKA3PNBmSnzyt77OQi2g\nMxHg+FqFDIZ36rDn32ECAwEAAaOBnjCBmzAOBgNVHQ8BAf8EBAMCA7gwHQYDVR0l\nBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMB0GA1UdDgQWBBQhFT4ikvcHNOTu/BRD\nlAcJ/dGWSTAfBgNVHSMEGDAWgBSEeJrL3GiaTrqlwqYxNoHToOg3eDAqBgNVHREE\nIzAhgh9QWEMgUmVsZWFzZSBUZXN0aW5nIENlcnRpZmljYXRlMA0GCSqGSIb3DQEB\nCwUAA4ICAQBSWN4WaELIsYYPsZg5JkwSzaB9obL/mIyyoWcI801IzLOleY3A7/rg\nD/JDqymnmciqwCCAF6Vq3C16jPXqNluHySxkNMUaJ6TTPqJHq/bs4GQjbs195fB9\nGv65+i5i+4CI+dHbmmBJz6JJXTRjurXIXX5BDZaYK6gu1yzskhwsGfcYG820p0vU\nXbwW1iH745Aw+2fHp/dvxo3LfJ8Dz5RoVf3wpWfHYxxwVmNpkXphWk7LD2VHJE+A\nYrsnQwfz5OyaSg1qdD7w1W4X7kGvGVQLl7sCtlknzd3eI27Vnsxw6ixAvpcakzLK\n7lPUHt6OYZzcdmqN3AjZWEGSVyzlrk7FU+dk/Fn3lchlpIHOcXxYLWDYnjiAkhYF\nwO4Bnwf524EFpI47CABmqwXINf+ivZvwqkCM2FIz+ANMYtMOb7fBe4wqs6pU8esE\nHMj0xf7z+IXKgSMBNZ4RT5oh1W4EdTGuMOgaZZNjy1rBvNQwDgjcZ3zXy4WOWYpI\nEpVicvfzpFKGAV7i0qUmPaxYI2b7fP2wkU4OVE+d4CHrMs3eAnOz1LXgVR65EzJk\nQ98UHv3CyKN6snZOjHcFHWnXRwd31PdyvEPNTtWyhJd3JOLQh5NOhx0wUkvjKZ1A\niIIwCVN1039vvYaBJE3DGQ3AfTlfPRvOjf+wMkxLTbWENu0w+KBHsQ==\n-----END CERTIFICATE-----",
						"PrivateKey": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAylU6WlXyftTNVk7NqsQMLSmPjw+paxVE7PuH5691a+TygjZW\nikCCHWHuXF9hEFVcHAW3yBRUU9zJIZpoEW816dnTPTuG30r3t6mHCzjO/6IUln11\ndte47o43PX+cSt/jOW+KPw8I00mrKGSlkZ9qmXvIy361OKipo+Ydgq/ODa8M53ak\ng6UboNvcs4wyRKTNp6yokFzyUAABUWiQi2l4Ky6JuyFb5+bQoallR3glrjHynKVs\n8kydR5uwKqwC/d9cdfNNfmPtOT02y919p9S/srLnkKv5kYQLHA7JHuKRQn/Kq1W4\noDc80GZKfPK3vs5CLaAzEeD4WoUMhnfqsOffYQIDAQABAoIBABRevD84A+/s8LdN\nu7sDfc5WLtPSWdYkEApj0Gzs5z9zj064tcG5eOAIafz2xUjlrx5LHRORqGPBCKbP\nasbGkYD3oyK5CX0ViRb+hFAG6iAmazgzbU4HreCmhL02Sb/oBnJT3nE2zEapVgf9\nCgY9LHhDVBXYrdoEcP/FPRdZJ7+MwDvussmyusEEims+6DBgfBfJ0n5/q5wrFVRY\nRqGBVTOkyoKdVDWl7ZyFP47nsWL5z7vXouFsZkcPBLVnKcTokiGjWJ0bHJSQMN35\nWe5HcWxKS58GpIZpwZdSNYL61INC7GS5p9wtXXuz11yznVhhHXiWnWVqRwtflBZU\nTg/YVgECgYEA7VnRzgCuyMRXlMPQ+EswcHCAkoO3m0YY4JHJSWI4yAxiR1mQa9PE\nQZajBc42PchgU9TFmKzK/x5mogpjNiIuvjs3EjMMShcJQ1+QkkMf7qjWAbOLHw5C\nZR7pYHsUoLVe6lo/PVbDeksvWYn7NHbeNZMxQU3Zapdjryk39Ao0r/ECgYEA2jsK\nxoK/hWNpYDu6KXk8AkIx0UiziVRLLEFiK2OL1vWrqWDfvyc4FR4A3D9R4UadwaLi\nSMjVDHNXhK5zgw74R4r+QkK9TfE1Nu2Sh510p0zraZN59Heu6HMa0qwER2Gd+sqv\nGL4JTouuevolS2jmRwwJDqGqarNuXTZvXIh/lnECgYEArbrK6QBy7x1YBdn2wWc3\nw3V1hsjGwe7jEq1jKkWszjDSVutl3Kcaxe9e0EcKSNq8N2BMad5Aj9BOy1jTGbKX\niEKLotSesDSAYUI37fcYDnaifohO0qJ7Usz3gdlVVfSrztnT5C/30THrLJzktJg1\nOf3NVcGH3I+HNZT0EbrOqeECgYBFZREAHwOX/wy7NUL1fT+/2BzPWDb/LHbbE8+L\nzJPjPyvfKJb9yhLjZC8R2nDHGhWARbN/QZ2938+suWyx+EirN1+y4lYgOtuZI+K8\njS1TJfqWD/dv7b8I29FjvJ9/s2LxJRKY45VCDNjm8jR5zlmrrgATTwVJ+NTXfz/a\nRO2NgQKBgBc/6VY4bfDjyd6cNhVWL98XMCl0e6w+0ltDMrr9fT7sVM2lIfO62WD3\nb23ZbJjnYkk9LDOoqI7CghmB98vrPQrSo5WsZPm8emWnuPiqVnN3hWnCpA+ismYh\n+fWZLdA9i5eWFZdRdgsDmtG/WeB81bo4SuHEgvw67M+u6zMlwutl\n-----END RSA PRIVATE KEY-----"
					}
				}
			}`

			osArgs := []string{
				"galera-healthcheck",
				fmt.Sprintf("-config=%s", rawConfig),
			}

			var err error
			rootConfig, err = NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("can provide a TLS listener", func() {
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

			conn, err := tls.Dial("tcp", "localhost:8080", &tls.Config{
				RootCAs:    certPool,
				ServerName: "PXC Release Testing Certificate",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(conn.Close()).To(Succeed())

			Eventually(errCh).Should(Receive(nil))
		})

		When("tls is disabled", func() {
			It("provides a plaintext TCP listener", func() {
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

				conn, err := net.Dial("tcp", "localhost:8080")
				Expect(err).NotTo(HaveOccurred())

				msg, err := ioutil.ReadAll(conn)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(msg)).To(Equal("foo"))
				Expect(conn.Close()).To(Succeed())
				Eventually(errCh).Should(Receive(nil))
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

	DescribeTable("IsHealthy",
		func(ls domain.WsrepLocalState, availableWhenDonor bool, availableWhenReadOnly bool, readOnly bool, expected bool) {
			config := &Config{
				AvailableWhenDonor:    availableWhenDonor,
				AvailableWhenReadOnly: availableWhenReadOnly,
			}

			state := domain.DBState{
				WsrepLocalState: ls,
				ReadOnly:        readOnly,
			}

			Expect(config.IsHealthy(state)).To(Equal(expected))
		},
		Entry("Joining is always false", domain.Joining, false, false, false, false),
		Entry("Joined is always false", domain.Joined, false, false, false, false),
		Entry("DonorDesynced when not availableWhenDonor is false ", domain.DonorDesynced, false, false, false, false),
		Entry("DonorDesynced when availableWhenReadOnly is always true - 1", domain.DonorDesynced, true, true, false, true),
		Entry("DonorDesynced when availableWhenReadOnly is always true - 2", domain.DonorDesynced, true, true, true, true),
		Entry("DonorDesynced when not availableWhenReadOnly is !readOnly - 1", domain.DonorDesynced, true, false, false, true),
		Entry("DonorDesynced when not availableWhenReadOnly is !readOnly - 2", domain.DonorDesynced, true, false, true, false),
		Entry("Synced when availableWhenReadOnly is always true - 1", domain.Synced, true, true, false, true),
		Entry("Synced when availableWhenReadOnly is always true - 2", domain.Synced, true, true, true, true),
		Entry("Synced when not availableWhenReadOnly is !readOnly - 1", domain.Synced, true, false, false, true),
		Entry("Synced when not availableWhenReadOnly is !readOnly - 2", domain.Synced, true, false, true, false),
	)
})
