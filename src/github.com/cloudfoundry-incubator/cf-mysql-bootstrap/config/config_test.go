package config_test

import (
	"fmt"
	"io/ioutil"
	"os"

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
		tmpFile    *os.File
	)

	BeforeEach(func() {
		var err error
		tmpFile, err = ioutil.TempFile("", "fake-logfile")
		Expect(err).NotTo(HaveOccurred())

		rawConfig = fmt.Sprintf(`{
				"HealthcheckURLs": [
					"10.10.10.10:9200",
					"11.11.11.11:9200",
					"12.12.12.12:9200"
				],
				"Username": "fake-username",
				"Password": "fake-password",
				"LogFilePath": "%s",
				"RepairMode": "bootstrap"
			}`, tmpFile.Name())

		osArgs = []string{
			"bootstrap",
			fmt.Sprintf("-config=%s", rawConfig),
		}
		rootConfig, err = NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(tmpFile.Name())
	})

	Describe("BuildLogger", func() {
		It("creates a logger with the logfile as a sink", func() {
			err := rootConfig.BuildLogger()
			Expect(err).NotTo(HaveOccurred())
			rootConfig.Logger.Info("fake log output")
			logBytes, err := ioutil.ReadFile(tmpFile.Name())
			Expect(err).NotTo(HaveOccurred())
			logged := string(logBytes)
			Expect(logged).To(ContainSubstring("fake log output"))
		})
	})

	Describe("Validate", func() {
		Context("valid config", func() {
			It("accepts bootstrap as a value for RepairMode", func() {
				err := rootConfig.Validate()
				Expect(err).NotTo(HaveOccurred())
			})

			It("accepts force-rejoin as a value for RepairMode", func() {
				rootConfig.RepairMode = "force-rejoin"
				err := rootConfig.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("returns an error if HealthcheckURLs is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "HealthcheckURLs")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if LogFilePath is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "LogFilePath")
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
