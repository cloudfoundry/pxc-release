package tls_test

import (
	"testing"

	helpers "specs/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTls(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tls Suite")
}

var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"MYSQL_HOST",
		"MYSQL_USERNAME",
		//"MYSQL_PASSWORD",
	}
	helpers.CheckForRequiredEnvVars(requiredEnvs)
})
