package connection_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"
)



var _ = Describe("CF PXC MySQL Connection", func() {

	BeforeEach(func() {
		helpers.DbSetup("connection_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup()
	})

	It("allows reading and writing data", func() {
		dbConn := helpers.DbConn()
		query := "INSERT INTO connection_test_table VALUES('connecting!')"
		_, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())


		var queryResultString string
		query = "SELECT * FROM connection_test_table"
		rows, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("connecting!"))
	})

})
