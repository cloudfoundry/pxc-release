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

// This test package is used as the pxc smoke-test, thus has different required
// variables than the rest of the integration test. In the future, we might
// want to move this test package somewhere else.
var _ = BeforeSuite(func() {
	requiredEnvs := []string{
		"MYSQL_HOST",
		"MYSQL_USERNAME",
		"MYSQL_PASSWORD",
	}

	helpers.CheckForRequiredEnvVars(requiredEnvs)

	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlUsername := os.Getenv("MYSQL_USERNAME")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	mysqlConn = helpers.DbConnWithUser(mysqlUsername, mysqlPassword, mysqlHost)
})
