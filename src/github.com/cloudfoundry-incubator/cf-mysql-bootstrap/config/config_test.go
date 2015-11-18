package config_test

import (
	"fmt"

	. "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
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
				"HealthcheckURLs": [
					"10.10.10.10:9200",
					"11.11.11.11:9200",
					"12.12.12.12:9200"
				]
			}`

			osArgs := []string{
				"bootstrap",
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

		It("returns an error if HealthcheckURLs is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "HealthcheckURLs")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
