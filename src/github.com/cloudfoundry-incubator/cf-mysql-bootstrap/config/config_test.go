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
				"ClusterIps": [
					"10.10.10.10",
					"11.11.11.11",
					"12.12.12.12"
				],
				"HealthcheckPort": 9200
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

		It("returns an error if ClusterIps is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "ClusterIps")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if HealthcheckPort is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "HealthcheckPort")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
