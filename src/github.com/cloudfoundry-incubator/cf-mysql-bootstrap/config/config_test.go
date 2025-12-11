package config_test

import (
	"fmt"

	. "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {

	var (
		rootConfig *Config
		rawConfig  string
		osArgs     []string
	)

	JustBeforeEach(func() {
		osArgs = []string{
			"bootstrap",
			fmt.Sprintf("-config=%s", rawConfig),
		}
		var err error
		rootConfig, err = NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Non-TLS Config", func() {
		BeforeEach(func() {
			rawConfig = `{
			"HealthcheckURLs": [
				"http://10.10.10.10:9200",
				"http://11.11.11.11:9200",
				"http://12.12.12.12:9200"
			],
			"Username": "fake-username",
			"Password": "fake-password",
			"RepairMode": "bootstrap"
			}`
		})

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
			rootConfig.HealthcheckURLs = []string{}
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HealthcheckURLs"))
		})

		It("returns an error if Username is blank", func() {
			rootConfig.Username = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Username"))
		})

		It("returns an error if Password is blank", func() {
			rootConfig.Password = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Password"))
		})

		It("returns an error if RepairMode is blank", func() {
			rootConfig.RepairMode = ""
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("RepairMode"))
		})

		It("returns an error if RepairMode is invalid", func() {
			rootConfig.RepairMode = "shoestrap"
			err := rootConfig.Validate()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("TLS Config", func() {
		When("BackendTLS params are provided", func() {
			BeforeEach(func() {
				rawConfig = `{
				"HealthcheckURLs": [
					"https://10.10.10.10:9200",
					"https://11.11.11.11:9200",
					"https://12.12.12.12:9200"
				],
				"Username": "fake-username",
				"Password": "fake-password",
				"RepairMode": "bootstrap",
				"BackendTLS": {
					"Enabled": true,
					"ServerName": "backendTlsServerName",
					"CA": "backendTlsCA",
					"InsecureSkipVerify": false
				}
			}`
			})
			It("accepts BackendTLS params in the config", func() {
				err := rootConfig.Validate()
				Expect(err).NotTo(HaveOccurred())
				Expect(rootConfig.BackendTLS.Enabled).To(BeTrue())
				Expect(rootConfig.BackendTLS.ServerName).To(Equal("backendTlsServerName"))
				Expect(rootConfig.BackendTLS.CA).To(Equal("backendTlsCA"))
				Expect(rootConfig.BackendTLS.InsecureSkipVerify).To(BeFalse())
			})
		})
		When("BackendTLS params aren't provided", func() {
			BeforeEach(func() {
				rawConfig = `{}`
			})
			It("configures the expected defaults", func() {
				Expect(rootConfig.BackendTLS.Enabled).To(BeFalse())
				Expect(rootConfig.BackendTLS.InsecureSkipVerify).To(BeFalse())
			})
		})
	})
})
