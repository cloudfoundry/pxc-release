package no_remote_access_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	helpers "specs/test_helpers"
	"testing"
)

func TestScaling(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Acceptance Tests -- No Remote Admin Access")
}

var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"BOSH_ENVIRONMENT",
		"MYSQL_USERNAME",
		"MYSQL_PASSWORD",
	}
	helpers.CheckForRequiredEnvVars(requiredEnvs)
})
