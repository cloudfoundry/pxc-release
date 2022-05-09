package config_test

import (
	"fmt"

	. "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {

	var (
		rootConfig *Config
		rawConfig  string
		osArgs     []string
	)

	BeforeEach(func() {
		rawConfig = `{
			"HealthcheckURLs": [
				"10.10.10.10:9200",
				"11.11.11.11:9200",
				"12.12.12.12:9200"
			],
			"Username": "fake-username",
			"Password": "fake-password",
			"RepairMode": "bootstrap"
		}`

		osArgs = []string{
			"bootstrap",
			fmt.Sprintf("-config=%s", rawConfig),
		}
		var err error
		rootConfig, err = NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Validate", func() {
		Context("valid config", func() {
			It("accepts bootstrap as a value for RepairMode", func() {
				err := rootConfig.Validate()
				Expect(err).NotTo(HaveOccurred())
			})

			It("accepts rejoin-unsafe as a value for RepairMode", func() {
				rootConfig.RepairMode = "rejoin-unsafe"
				err := rootConfig.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("returns an error if HealthcheckURLs is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "HealthcheckURLs")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Username is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Username")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if RepairMode is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "RepairMode")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if RepairMode is invalid", func() {
			rootConfig.RepairMode = "shoestrap"
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
		})

	})
})
