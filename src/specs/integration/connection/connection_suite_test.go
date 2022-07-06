package connection_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
)

func TestConnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Acceptance Tests -- Connection")
}

var (
	mysqlConn     *sql.DB
	mysqlHosts    []string
	mysqlUsername string
	mysqlPassword string
)

// This test package is used as the pxc smoke-test, thus has different required
// variables than the rest of the integration test. In the future, we might
// want to move this test package somewhere else.
var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"MYSQL_USERNAME",
		"MYSQL_PASSWORD",
		"MYSQL_HOSTS",
		"PROXY_HOST",
	}

	helpers.CheckForRequiredEnvVars(requiredEnvs)

	proxyHost := os.Getenv("PROXY_HOST")
	mysqlHosts = strings.Fields(os.Getenv("MYSQL_HOSTS"))
	mysqlUsername = os.Getenv("MYSQL_USERNAME")
	mysqlPassword = os.Getenv("MYSQL_PASSWORD")
	mysqlConn = helpers.DbConnWithUser(mysqlUsername, mysqlPassword, proxyHost)
})
