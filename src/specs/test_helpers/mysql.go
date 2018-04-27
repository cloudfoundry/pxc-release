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
		DbHost(),
		3306)
	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	statement := "CREATE DATABASE IF NOT EXISTS pxc_release_test_db"
	_, err = databaseConnection.Exec(statement)
	Expect(err).NotTo(HaveOccurred())

	statement = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (test_data varchar(255) PRIMARY KEY)", tableName)
	_, err = DbConn().Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}

func DbConnNoDb() *sql.DB {
	var mysqlUsername = os.Getenv("MYSQL_USERNAME")
	var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

	pxcConnectionString := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/",
		mysqlUsername,
		mysqlPassword,
		DbHost(),
		3306)

	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	return databaseConnection
}

func DbConn() *sql.DB {
	var mysqlUsername = os.Getenv("MYSQL_USERNAME")
	var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

	pxcConnectionString := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/pxc_release_test_db",
		mysqlUsername,
		mysqlPassword,
		DbHost(),
		3306)

	databaseConnection, err := sql.Open("mysql", pxcConnectionString)
	Expect(err).NotTo(HaveOccurred())

	return databaseConnection
}

func DbCleanup() {
	statement := "DROP DATABASE pxc_release_test_db"
	_, err := DbConn().Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}

func DbHost() string {
	dbHost, hostExists := os.LookupEnv("MYSQL_HOST")
	if hostExists {
		return dbHost
	}
	return os.Getenv("BOSH_ENVIRONMENT")
}
