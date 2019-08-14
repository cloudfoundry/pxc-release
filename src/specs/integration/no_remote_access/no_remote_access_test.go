package no_remote_access_test

import (
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CF PXC No Remote Admin Access", func() {
	It("does not allow access to mysql from anywhere besides localhost", func() {
		query := "show variables"
		_, err := mysqlConn.Query(query)
		Expect(err).To(MatchError(MatchRegexp("is not allowed to connect to this MySQL server|Access denied for user")))
	})
})
