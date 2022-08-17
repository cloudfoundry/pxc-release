package config_test

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/pivotal-cf-experimental/service-config"

	. "github.com/cloudfoundry/galera-init/config"
)

var _ = Describe("Config", func() {

	Describe("Validate", func() {
		var rootConfig Config
		var serviceConfig *ServiceConfig

		BeforeEach(func() {
			serviceConfig = New()
			flags := flag.NewFlagSet("galera-init", flag.ExitOnError)
			serviceConfig.AddFlags(flags)

			serviceConfig.AddDefaults(Config{
				Db: DBHelper{
					User: "root",
				},
			})

			flags.Parse([]string{
				"-configPath=../example-config.yml",
			})

			err := serviceConfig.Read(&rootConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		var setNestedFieldToEmpty func(obj interface{}, nestedFieldNames []string) error
		setNestedFieldToEmpty = func(obj interface{}, nestedFieldNames []string) error {

			s := reflect.ValueOf(obj).Elem()
			if s.Type().Kind() == reflect.Slice {
				if s.Len() == 0 {
					return errors.New("Trying to set nested property on empty slice")
				}
				s = s.Index(0)
			}

			currFieldName := nestedFieldNames[0]
			remainingFieldNames := nestedFieldNames[1:]
			field := s.FieldByName(currFieldName)
			if field.IsValid() == false {
				return errors.New(fmt.Sprintf("Field '%s' is not defined", currFieldName))
			}

			if len(remainingFieldNames) == 0 {
				fieldType := field.Type()
				field.Set(reflect.Zero(fieldType))
				return nil
			}
			return setNestedFieldToEmpty(field.Addr().Interface(), remainingFieldNames)
		}

		var setFieldToEmpty = func(fieldName string) error {
			return setNestedFieldToEmpty(&rootConfig, strings.Split(fieldName, "."))
		}

		var isRequiredField = func(fieldName string) func() {
			return func() {
				err := setFieldToEmpty(fieldName)
				Expect(err).NotTo(HaveOccurred())

				err = rootConfig.Validate()

				Expect(err).To(HaveOccurred())

				fieldParts := strings.Split(fieldName, ".")
				for _, fieldPart := range fieldParts {
					Expect(err.Error()).To(ContainSubstring(fieldPart))
				}
			}
		}

		var isOptionalField = func(fieldName string) func() {
			return func() {
				err := setFieldToEmpty(fieldName)
				Expect(err).NotTo(HaveOccurred())

				err = rootConfig.Validate()

				Expect(err).NotTo(HaveOccurred())
			}
		}

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()

			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Config", func() {
			It("returns an error if LogFileLocation is blank", isRequiredField("LogFileLocation"))
		})

		Describe("StartManager", func() {
			It("returns an error if Manager.StateFileLocation is blank", isRequiredField("Manager.StateFileLocation"))
			It("returns an error if Manager.ClusterIps is blank", isRequiredField("Manager.ClusterIps"))
			It("returns an error if Manager.ClusterProbeTimeout is blank", isRequiredField("Manager.ClusterProbeTimeout"))
		})

		Describe("DBHelper", func() {
			It("returns an error if Db.User is blank", isRequiredField("Db.User"))

			It("does not return an error if Db.Password is blank", isOptionalField("Db.Password"))
		})
	})

	Describe("HTTPClient", func() {
		var rootConfig *Config
		BeforeEach(func() {
			osArgs := []string{
				"galera-init",
				"-configPath=fixtures/validConfig.yml",
			}

			var err error
			rootConfig, err = NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("provides an http client for communicating with galera-agent", func() {
			httpClient := rootConfig.HTTPClient()

			Expect(rootConfig.Manager.ClusterProbeTimeout).To(BeEquivalentTo(13))

			Expect(httpClient).NotTo(BeZero())
			Expect(httpClient.Timeout).To(Equal(time.Duration(rootConfig.Manager.ClusterProbeTimeout) * time.Second))
		})

		When("the proxy timeout is configured differently", func() {
			BeforeEach(func() {
				rootConfig.Manager.ClusterProbeTimeout = 42
			})

			It("configures the http client timeout with Config.Manager.ClusterProbeTimeout", func() {
				httpClient := rootConfig.HTTPClient()

				Expect(httpClient.Timeout).To(Equal(time.Duration(rootConfig.Manager.ClusterProbeTimeout) * time.Second))
			})
		})

		When("Galera Agent TLS is not enabled", func() {
			BeforeEach(func() {
				rootConfig.BackendTLS.Enabled = false
			})

			It("does not configure a TLSClientConfig", func() {
				httpClient := rootConfig.HTTPClient()

				Expect(httpClient.Transport).To(BeNil())
			})
		})

		When("Galera Agent TLS is enabled", func() {
			BeforeEach(func() {
				rootConfig.BackendTLS.Enabled = true
			})

			It("configures a TLSClientConfig", func() {
				httpClient := rootConfig.HTTPClient()

				Expect(httpClient.Transport).To(BeAssignableToTypeOf(&http.Transport{}))

				transport := httpClient.Transport.(*http.Transport)

				Expect(transport.TLSClientConfig.ServerName).To(Equal(rootConfig.BackendTLS.ServerName))
				Expect(transport.TLSClientConfig.RootCAs).NotTo(BeNil()) // doesn't look like we can inspect the CA
			})
		})
	})

	Describe("ClusterUrls", func() {
		var rootConfig *Config
		BeforeEach(func() {
			osArgs := []string{
				"galera-init",
				"-configPath=fixtures/validConfig.yml",
			}

			var err error
			rootConfig, err = NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("generates URLs from IPs", func() {
			urls := rootConfig.ClusterUrls()

			expected := []string{
				"http://1.1.1.1:9200/",
				"https://1.1.1.1:9201/",
				"http://1.1.1.2:9200/",
				"https://1.1.1.2:9201/",
				"http://1.1.1.3:9200/",
				"https://1.1.1.3:9201/",
			}
			Expect(urls).To(ConsistOf(expected))
		})
	})
})
