package test_helpers

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/gomega"
	"os"
)

func DbSetup(tableName string) {
	var mysqlUsername = os.Getenv("MYSQL_USERNAME")
	var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

	pxcConnectionString := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/",
		mysqlUsername,
		mysqlPassword,
		BoshEnvironment(),
		3306)
	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	statement := "CREATE DATABASE IF NOT EXISTS integration_test"
	_, err = databaseConnection.Exec(statement)
	Expect(err).NotTo(HaveOccurred())

	statement = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (test_data varchar(255))", tableName)
	_, err = DbConn().Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}

func DbConn() *sql.DB {
	var mysqlUsername = os.Getenv("MYSQL_USERNAME")
	var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

	pxcConnectionString := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/integration_test",
		mysqlUsername,
		mysqlPassword,
		BoshEnvironment(),
		3306)

	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	return databaseConnection
}

func DbCleanup() {
	statement := "DROP DATABASE integration_test"
	_, err := DbConn().Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}
