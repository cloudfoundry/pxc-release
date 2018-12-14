package test_helpers

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/gomega"
)

func DbSetup(db *sql.DB, tableName string) string {
	var (
		dbName = "pxc_release_test_db"
		err    error
	)

	_, err = db.Exec(`CREATE DATABASE IF NOT EXISTS pxc_release_test_db`)
	Expect(err).NotTo(HaveOccurred())

	statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS pxc_release_test_db.%s (test_data varchar(255) PRIMARY KEY)", tableName)
	_, err = db.Exec(statement)
	Expect(err).NotTo(HaveOccurred())
	return dbName
}

func DbConnWithUser(mysqlUsername, mysqlPassword, mysqlHost string) *sql.DB {
	pxcConnectionString := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/?tls=skip-verify",
		mysqlUsername,
		mysqlPassword,
		mysqlHost,
		3306)

	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	return databaseConnection
}

func DbCleanup(db *sql.DB) {
	statement := "DROP DATABASE pxc_release_test_db"
	_, err := db.Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}
