package download_logs_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
)

func TestDownloadLogs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DownloadLogs Suite")
}

var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"BOSH_DEPLOYMENT",
	}
	helpers.CheckForRequiredEnvVars(requiredEnvs)

	helpers.SetupBoshDeployment()
	if os.Getenv("BOSH_ALL_PROXY") != "" {
		helpers.SetupSocks5Proxy()
	}
})
