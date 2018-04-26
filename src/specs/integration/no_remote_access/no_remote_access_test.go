package no_remote_access

import (
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"
)

var _ = Describe("CF PXC No Remote Admin Access", func() {

	It("does not allow access to mysql from anywhere besides localhost", func() {
		databaseConnection := helpers.DbConnNoDb()

		query := "show variables"
		_, err := databaseConnection.Query(query)
		Expect(err).To(MatchError(MatchRegexp("is not allowed to connect to this MySQL server|Access denied for user")))
	})

})
