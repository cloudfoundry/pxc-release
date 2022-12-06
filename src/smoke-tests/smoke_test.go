package smoke_test

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Smoke Test Database Connectivity", func() {
	var db *sql.DB

	openDB := func(host string) (*sql.DB, error) {
		var (
			mysqlUser     = os.Getenv("MYSQL_USERNAME")
			mysqlPassword = os.Getenv("MYSQL_PASSWORD")
		)
		dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/?tls=preferred&wsrep_sync_wait=1",
			mysqlUser, mysqlPassword, host)
		return sql.Open("mysql", dsn)
	}

	testConnection := func(db *sql.DB) (string, error) {
		var data string
		err := db.QueryRow("SELECT * FROM pxc_release_test_db.connection_test_table").
			Scan(&data)
		return data, err
	}

	BeforeEach(func() {
		var err error
		db, err = openDB(os.Getenv("PROXY_HOST"))
		Expect(err).NotTo(HaveOccurred())

		Expect(db.Exec("CREATE DATABASE IF NOT EXISTS pxc_release_test_db")).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec("CREATE TABLE IF NOT EXISTS pxc_release_test_db.connection_test_table(test_data varchar(255) PRIMARY KEY)")).
			Error().NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(db.Exec("DROP DATABASE pxc_release_test_db")).
			Error().NotTo(HaveOccurred())
	})

	It("allows reading and writing data", func() {
		Expect(db.Exec("INSERT INTO pxc_release_test_db.connection_test_table VALUES('connecting!')")).
			Error().NotTo(HaveOccurred())

		Expect(testConnection(db)).To(Equal("connecting!"),
			`Expected to read back data through the proxy connection, but this failed`)
		for _, host := range strings.Fields(os.Getenv("MYSQL_HOSTS")) {
			db, err := openDB(host)
			Expect(err).NotTo(HaveOccurred())
			Expect(testConnection(db)).To(Equal("connecting!"),
				`Expected to read back data from node %q, but this failed`, host)
		}
	})
})
