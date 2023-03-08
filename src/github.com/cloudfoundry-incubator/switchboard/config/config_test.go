package config_test

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	. "github.com/cloudfoundry-incubator/switchboard/config"
)

var _ = Describe("Config", func() {
	Describe("Proxy methods", func() {
		Describe("HealthcheckTimeout", func() {
			It("returns timeout in millis", func() {
				Expect(Proxy{HealthcheckTimeoutMillis: 10}.HealthcheckTimeout()).To(Equal(10 * time.Millisecond))
			})
		})

		Describe("ShutdownDelay", func() {
			It("returns delay in seconds", func() {
				Expect(Proxy{ShutdownDelaySeconds: 10}.ShutdownDelay()).To(Equal(10 * time.Second))
			})
		})
	})

	Describe("Validate", func() {
		var (
			rootConfig    *Config
			rawConfigFile string
		)

		BeforeEach(func() {
			rawConfigFile = "fixtures/validConfig.yml"
			osArgs := []string{
				"switchboard",
				fmt.Sprintf("-configPath=%s", rawConfigFile),
			}

			var err error
			rootConfig, err = NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		When("GaleraAgentTLS is enabled", func() {

			var (
				caPEMBytes []byte
				err        error
			)

			BeforeEach(func() {
				rootConfig.GaleraAgentTLS.Enabled = true
			})

			JustBeforeEach(func() {
				err = rootConfig.Validate()
			})

			When("GaleraAgentTLS.CA is valid", func() {
				BeforeEach(func() {
					serverAuthority, err := certtest.BuildCA("test")

					Expect(err).ToNot(HaveOccurred())
					caPEMBytes, err = serverAuthority.CertificatePEM()

					Expect(err).ToNot(HaveOccurred())
					rootConfig.GaleraAgentTLS.CA = string(caPEMBytes)

				})
				It("does not throw an error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("GaleraAgentTLS.CA is invalid", func() {
				It("returns an error", func() {
					Expect(err).To(MatchError(errors.New(fmt.Sprintf("Validation errors: %s\n", fmt.Sprintf("%s%s : %s\n", "", "GaleraAgentTLS.CA", "Failed to Parse CA.")))))
				})
			})

		})
		When("SwitchboardApiTLS is enabled", func() {

			var (
				err error
			)

			BeforeEach(func() {
				rootConfig.API.TLS.Enabled = true
			})

			JustBeforeEach(func() {
				err = rootConfig.Validate()
			})

			When("SwitchboardApi Certificate and PrivateKey are valid", func() {
				BeforeEach(func() {
					authority, err := certtest.BuildCA("testCA")
					Expect(err).NotTo(HaveOccurred())

					certificate, err := authority.BuildSignedCertificate("localhost")
					Expect(err).NotTo(HaveOccurred())

					caPEMBytes, privateKeyBytes, err := certificate.CertificatePEMAndPrivateKey()
					Expect(err).NotTo(HaveOccurred())

					rootConfig.API.TLS.Certificate = string(caPEMBytes)
					rootConfig.API.TLS.PrivateKey = string(privateKeyBytes)

				})
				It("does not throw an error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("SwitchboardApi Certificate and PrivateKey are invalid", func() {
				It("returns an error", func() {
					Expect(err).To(MatchError(errors.New(fmt.Sprintf("Validation errors: %s\n", fmt.Sprintf("%s%s : %s\n", "", "SwitchboardApi.Certificate/PrivateKey", "Failed to Parse Certificate or PrivateKey.")))))
				})
			})
		})

		It("configures GaleraAgentTLS properties", func() {
			Expect(rootConfig.GaleraAgentTLS.Enabled).To(BeFalse(),
				`Expected fixtures/validConfig.yml to unmarshal a GaleraAgentTLS.Enabled = true property, but it did not.  Are the struct tags correct?`)
			Expect(rootConfig.GaleraAgentTLS.CA).To(Equal("this-should-be-a-PEM-encoded-CA"),
				`Expected fixtures/validConfig.yml to unmarshal the correct GaleraAgentTLS.CA property, but it did not.  Are the struct tags correct?`)
			Expect(rootConfig.GaleraAgentTLS.ServerName).To(Equal(`Expected server certificate identity`),
				`Expected fixtures/validConfig.yml to unmarshal the correct GaleraAgentTLS.ServerName property, but it did not.  Are the struct tags correct?`)
		})

		It("configures Proxy TLS properties", func() {
			Expect(rootConfig.API.TLS.Certificate).To(Equal(`some-pem-encoded-certificate`),
				`Expected fixtures/validConfig.yml to unmarshal the correct API.TLS.Certificate property, but it did not.  Are the struct tags correct?`)
			Expect(rootConfig.API.TLS.PrivateKey).To(Equal(`some-pem-encoded-private-key`),
				`Expected fixtures/validConfig.yml to unmarshal the correct API.TLS.Certificate property, but it did not.  Are the struct tags correct?`)
		})
		It("returns an error if API.Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "API.Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if API.Username is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "API.Username")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if API.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "API.Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not return an error if API.ForceHttps is blank", func() {
			err := test_helpers.IsOptionalField(rootConfig, "API.ForceHttps")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.HealthcheckTimeoutMillis is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.HealthcheckTimeoutMillis")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends.Host is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends.Host")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends.Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends.Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends.StatusPort is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends.StatusPort")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends.StatusEndpoint is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends.StatusEndpoint")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Proxy.Backends.Name is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Proxy.Backends.Name")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if HealthPort is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "HealthPort")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if StaticDir is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "StaticDir")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("HTTPClient", func() {
		var rootConfig *Config
		BeforeEach(func() {
			osArgs := []string{
				"switchboard",
				"-configPath=fixtures/validConfig.yml",
			}

			var err error
			rootConfig, err = NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("provides an http client for communicating with galera-agent", func() {
			httpClient := rootConfig.HTTPClient()
			Expect(httpClient).NotTo(BeZero())
			Expect(rootConfig.Proxy.HealthcheckTimeoutMillis).To(BeEquivalentTo(5000))
			Expect(httpClient.Timeout).To(Equal(time.Duration(rootConfig.Proxy.HealthcheckTimeoutMillis) * time.Millisecond))
		})

		When("the proxy timeout is configured differently", func() {
			BeforeEach(func() {
				rootConfig.Proxy.HealthcheckTimeoutMillis = 42
			})

			It("configures the http client timeout with Config.Proxy.HealthcheckTimeoutMillis", func() {
				httpClient := rootConfig.HTTPClient()
				Expect(httpClient.Timeout).To(Equal(42 * time.Millisecond))
			})
		})

		When("Galera Agent TLS is not enabled", func() {
			BeforeEach(func() {
				rootConfig.GaleraAgentTLS.Enabled = false
			})

			It("does not configure a TLSClientConfig", func() {
				httpClient := rootConfig.HTTPClient()

				Expect(httpClient.Transport).To(BeNil())
			})
		})

		When("Galera Agent TLS is enabled", func() {
			BeforeEach(func() {
				rootConfig.GaleraAgentTLS.Enabled = true
			})

			It("configures a TLSClientConfig", func() {
				httpClient := rootConfig.HTTPClient()

				Expect(httpClient.Transport).To(BeAssignableToTypeOf(&http.Transport{}))

				transport := httpClient.Transport.(*http.Transport)
				Expect(transport.TLSClientConfig).NotTo(BeNil())
			})
		})
	})
})
