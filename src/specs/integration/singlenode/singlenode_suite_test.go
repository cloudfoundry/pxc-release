package singlenode_test

import (
	"database/sql"
	"os"
	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSingleNode(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Acceptance Tests -- SingleNode")
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
	mysqlHosts, err := helpers.MySQLHosts(helpers.BoshDeployment)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(mysqlHosts)).To(Equal(1))
	mysqlConn = helpers.DbConnWithUser(mysqlUsername, mysqlPassword, mysqlHosts[0])
})
