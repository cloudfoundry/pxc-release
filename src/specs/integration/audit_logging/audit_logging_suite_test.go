package audit_logging_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	helpers "specs/test_helpers"
	"testing"
)

func TestAuditLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Acceptance Tests -- Audit Logging")
}

var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT_PATH",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"BOSH_DEPLOYMENT",
		"MYSQL_USERNAME",
		"MYSQL_PASSWORD",
		"AUDIT_LOG_PATH",
	}
	helpers.CheckForRequiredEnvVars(requiredEnvs)
})
