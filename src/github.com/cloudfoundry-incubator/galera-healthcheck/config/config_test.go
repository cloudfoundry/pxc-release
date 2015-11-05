package config_test

import (
	"fmt"

	. "github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
				"Port": "8080",
				"AvailableWhenReadOnly": false,
				"AvailableWhenDonor": true,
				"DB": {
					"Host": "localhost",
					"User": "vcap",
					"Port": 3000,
					"Password": "password"
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

		It("returns an error if StatusEndpoint is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "StatusEndpoint")
			Expect(err).ToNot(HaveOccurred())
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

		It("returns an error if DB.Host is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.Host")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.User is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.User")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.Port is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if DB.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "DB.Password")
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
