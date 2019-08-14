package connection_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"
)

var _ = Describe("CF PXC MySQL Connection", func() {

	BeforeEach(func() {
		helpers.DbSetup(mysqlConn, "connection_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup(mysqlConn)
	})

	It("allows reading and writing data", func() {
		query := "INSERT INTO pxc_release_test_db.connection_test_table VALUES('connecting!')"
		_, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		var queryResultString string
		query = "SELECT * FROM pxc_release_test_db.connection_test_table"
		rows, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("connecting!"))
	})

})
