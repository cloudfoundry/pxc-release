package integration_test

import (
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

	serviceConfig.AddDefaults(TestDBConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "",
	})

	err := serviceConfig.Read(&testConfig)
	Expect(err).NotTo(HaveOccurred())
})
