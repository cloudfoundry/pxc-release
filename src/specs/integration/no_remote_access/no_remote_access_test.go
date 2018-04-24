package no_remote_access

import (
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"database/sql"
	"fmt"
	"os"
	helpers "specs/test_helpers"
)

var _ = Describe("CF PXC No Remote Admin Access", func() {

	It("does not allow access to mysql from anywhere besides localhost", func() {
		var mysqlUsername = os.Getenv("MYSQL_USERNAME")
		var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

		pxcConnectionString := fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/",
			mysqlUsername,
			mysqlPassword,
			helpers.DbHost(),
			3306)
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		query := "show variables"
		_, err = databaseConnection.Query(query)
		Expect(err).To(MatchError(MatchRegexp("is not allowed to connect to this MySQL server|Access denied for user")))
	})

})
