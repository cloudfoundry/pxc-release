package connection_test

import (
	"database/sql"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
)

var _ = Describe("CF PXC MySQL Connection", func() {

	BeforeEach(func() {
		helpers.DbSetup(mysqlConn, "connection_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup(mysqlConn)
	})

	It("allows reading and writing data", func() {
		_, err := mysqlConn.Exec("INSERT INTO pxc_release_test_db.connection_test_table VALUES('connecting!')")
		Expect(err).NotTo(HaveOccurred())

		Expect(testConnection(mysqlConn)).To(Equal("connecting!"))

		for _, host := range mysqlHosts {
			conn := helpers.DbConnWithUser(mysqlUsername, mysqlPassword, host)
			EventuallyWithOffset(1, func() string {
				return testConnection(conn)
			}).Should(Equal("connecting!"))
		}
	})

})

func testConnection(conn *sql.DB) string {
	var queryResultString string
	query := "SELECT * FROM pxc_release_test_db.connection_test_table"
	rows, err := conn.Query(query)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	rows.Next()
	rows.Scan(&queryResultString)

	return queryResultString

}
