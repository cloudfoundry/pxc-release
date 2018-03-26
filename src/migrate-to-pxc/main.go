package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"

	"os/exec"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	mysqlAdminUsername := os.Getenv("MYSQL_USERNAME")
	mysqlAdminPassword := os.Getenv("MYSQL_PASSWORD")

	fmt.Println("starting mysql servers...")
	//Start mariadb
	mariadbCmd := exec.Command("/var/vcap/packages/mariadb/bin/mysqld_safe", "--defaults-file=/var/vcap/jobs/mysql/config/my.cnf", "--wsrep-on=OFF", "--wsrep-desync=ON", "--wsrep-OSU-method=RSU", "--wsrep-provider='none'", "--skip-networking")
	err := mariadbCmd.Start()
	if err != nil {
		panic(err)
	}

	dsn := fmt.Sprintf("%s:%s@unix(%s)/", mysqlAdminUsername, mysqlAdminPassword, "/var/vcap/sys/run/mysql/mysqld.sock")

	var mariadbDatabaseConnection *sql.DB

	for tries := 0; tries < 20; tries++ {
		mariadbDatabaseConnection, err = sql.Open("mysql", dsn)
		if err == nil {
			break
		}

		if tries == 19 {
			panic(err)
		}
		time.Sleep(5 * time.Second)
	}

	fmt.Println("retrieving databases...")
	// Get all the database names
	query := "select schema_name from information_schema.schemata where schema_name NOT IN ('performance_schema', 'mysql', 'information_schema')"

	rows, err := mariadbDatabaseConnection.Query(query)
	if err != nil {
		panic(err)
	}

	var databaseNames []string
	for rows.Next() {
		var databaseName string
		rows.Scan(&databaseName)
		databaseNames = append(databaseNames, databaseName)
	}

	fmt.Println("migrating data...")

	dumpArgs := []string{
		"/var/vcap/packages/pxc/bin/mysqldump",
		"--defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf",
		"--verbose",
		"--databases",
	}

	dumpArgs = append(dumpArgs, databaseNames...)

	loadArgs := []string{
		"/var/vcap/packages/pxc/bin/mysql",
		"--defaults-file=/var/vcap/jobs/mysql-clustered/config/mylogin.cnf",
		"--verbose",
	}

	dumpCmd := exec.Command(dumpArgs[0], dumpArgs[1:]...)
	loadCmd := exec.Command(loadArgs[0], loadArgs[1:]...)

	pr, pw := io.Pipe()
	dumpCmd.Stdout = pw
	loadCmd.Stdin = pr

	dumpCmd.Stderr = os.Stderr
	loadCmd.Stderr = os.Stderr
	loadCmd.Stdout = os.Stdout

	if err := dumpCmd.Start(); err != nil {
		panic(err)
	}

	if err := loadCmd.Start(); err != nil {
		panic(err)
	}

	go func() {
		defer pw.Close()

		if err := dumpCmd.Wait(); err != nil {
			panic(err)
		}
	}()

	if err := loadCmd.Wait(); err != nil {
		panic(err)
	}

	fmt.Println("stopping mariadb...")

	mariadbShutdownCmd := exec.Command("/var/vcap/packages/mariadb/support-files/mysql.server", "stop", "--pid-file=/var/vcap/sys/run/mysql/mysql.pid")
	out, err := mariadbShutdownCmd.CombinedOutput()
	if err != nil {
		println(err.Error())
		println(string(out))
		panic(err)
	}
}
