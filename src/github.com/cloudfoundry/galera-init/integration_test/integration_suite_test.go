package integration_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config"
)

var testConfig TestDBConfig

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Test Suite")
}

type TestDBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

var _ = BeforeSuite(func() {

	serviceConfig := service_config.New()

	//Use default options rather than throw error if env variables are blank
	if os.Getenv("CONFIG") == "" && os.Getenv("CONFIG_PATH") == "" {
		os.Setenv("CONFIG", "{}")
	}
	serviceConfig.AddDefaults(TestDBConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "",
	})

	err := serviceConfig.Read(&testConfig)
	Expect(err).NotTo(HaveOccurred())
})
