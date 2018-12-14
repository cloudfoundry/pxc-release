package connection_test

import (
	"database/sql"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"
)

func TestConnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Acceptance Tests -- Connection")
}

var (
	mysqlConn *sql.DB
)

var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"BOSH_DEPLOYMENT",
		"AUDIT_LOG_PATH",
		"CREDHUB_SERVER",
		"CREDHUB_CLIENT",
		"CREDHUB_SECRET",
	}

	helpers.CheckForRequiredEnvVars(requiredEnvs)

	helpers.SetupBoshDeployment()

	if os.Getenv("BOSH_ALL_PROXY") != "" {
		helpers.SetupSocks5Proxy()
	}

	mysqlUsername := "root"
	mysqlPassword, err := helpers.GetMySQLAdminPassword()
	Expect(err).NotTo(HaveOccurred())
	firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
	Expect(err).NotTo(HaveOccurred())
	mysqlConn = helpers.DbConnWithUser(mysqlUsername, mysqlPassword, firstProxy)
})
