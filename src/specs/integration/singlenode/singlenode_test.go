package singlenode_test

import (
	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CF PXC Single Node", func() {
	AfterEach(func() {
		_, err := mysqlConn.Query("RESET MASTER")
		Expect(err).NotTo(HaveOccurred())
	})

	It("has an empty GTID transaction history on startup", func() {
		var queryResultString string
		query := "select @@GLOBAL.gtid_executed;"
		row := mysqlConn.QueryRow(query)
		Expect(row.Scan(&queryResultString)).To(Succeed())
		Expect(queryResultString).To(BeEmpty())

		helpers.DbSetup(mysqlConn, "binarylogs")

		row = mysqlConn.QueryRow(query)
		Expect(row.Scan(&queryResultString)).To(Succeed())
		Expect(queryResultString).ToNot(BeEmpty())
	})
})
