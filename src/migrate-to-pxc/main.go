package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"

	"os/exec"
	"time"

	"github.com/cloudfoundry/gosigar"
	_ "github.com/go-sql-driver/mysql"
	"migrate-to-pxc/disk"
)

var (
	err error
)

func main() {
	// Create a Sigar to gather system info
	concreteSigar := sigar.ConcreteSigar{}
	err = disk.RoomToMigrate(&concreteSigar)

	if err != nil {
		panic(err)
	}

	mysqlAdminUsername := os.Getenv("MYSQL_USERNAME")
	mysqlAdminPassword := os.Getenv("MYSQL_PASSWORD")

	fmt.Println("starting mysql servers...")

	err := startMariaDB()
	if err != nil {
		panic(err)
	}

	mariadbDatabaseConnection, err := connectToMariaDB(mysqlAdminUsername, mysqlAdminPassword)
	if err != nil {
		panic(err)
	}

	databaseNames, err := listDBs(mariadbDatabaseConnection)

	fmt.Println("migrating data...")

	pr, pw := io.Pipe()

	mariaDBDump := mariaDBDumpCmd(databaseNames, pw)
	pxcLoad := pxcLoadCmd(pr)

	if err := mariaDBDump.Start(); err != nil {
		panic(err)
	}

	if err := pxcLoad.Start(); err != nil {
		panic(err)
	}

	go func() {
		defer pw.Close()

		if err := mariaDBDump.Wait(); err != nil {
			panic(err)
		}
	}()

	if err := pxcLoad.Wait(); err != nil {
		panic(err)
	}

	shutdownMariaDB()
}

func shutdownMariaDB() {
	fmt.Println("stopping mariadb...")
	mariadbShutdownCmd := exec.Command("/var/vcap/packages/mariadb/support-files/mysql.server", "stop", "--pid-file=/var/vcap/sys/run/mysql/mysql.pid")
	out, err := mariadbShutdownCmd.CombinedOutput()
	if err != nil {
		println(err.Error())
		println(string(out))
		panic(err)
	}
}

func pxcLoadCmd(in *io.PipeReader) *exec.Cmd {
	loadArgs := []string{
		"/var/vcap/packages/pxc/bin/mysql",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf",
	}
	loadCmd := exec.Command(loadArgs[0], loadArgs[1:]...)
	loadCmd.Stdin = in
	loadCmd.Stderr = os.Stderr
	loadCmd.Stdout = os.Stdout
	return loadCmd
}

func mariaDBDumpCmd(databaseNames []string, out *io.PipeWriter) *exec.Cmd {
	dumpArgs := []string{
		"/var/vcap/packages/pxc/bin/mysqldump",
		"--defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf",
		"--databases",
	}
	dumpArgs = append(dumpArgs, databaseNames...)
	dumpCmd := exec.Command(dumpArgs[0], dumpArgs[1:]...)
	dumpCmd.Stdout = out
	dumpCmd.Stderr = os.Stderr

	return dumpCmd
}

func listDBs(databaseConnection *sql.DB) ([]string, error) {
	fmt.Println("retrieving databases...")
	// Get all the database names
	var (
		rows *sql.Rows
		err  error
	)

	query := "select schema_name from information_schema.schemata where schema_name NOT IN ('performance_schema', 'mysql', 'information_schema')"

	for tries := 0; tries < 20; tries++ {
		rows, err = databaseConnection.Query(query)
		if err == nil {
			break
		}

		if tries == 19 {
			return nil, err
		}
		time.Sleep(5 * time.Second)
	}

	var databaseNames []string
	for rows.Next() {
		var databaseName string
		rows.Scan(&databaseName)
		databaseNames = append(databaseNames, databaseName)
	}
	return databaseNames, nil
}

func connectToMariaDB(mysqlAdminUsername, mysqlAdminPassword string) (*sql.DB, error) {
	mariadbConnectionString := fmt.Sprintf("%s:%s@unix(%s)/", mysqlAdminUsername, mysqlAdminPassword, "/var/vcap/sys/run/mysql/mysqld.sock")
	var mariadbDatabaseConnection *sql.DB
	for tries := 0; tries < 20; tries++ {
		mariadbDatabaseConnection, err = sql.Open("mysql", mariadbConnectionString)
		if err == nil {
			break
		}

		if tries == 19 {
			return nil, err
		}
		time.Sleep(5 * time.Second)
	}
	return mariadbDatabaseConnection, nil
}

func startMariaDB() error {
	_, err := os.Stat("/var/vcap/packages/mariadb/bin")
	if os.IsNotExist(err) {
		return fmt.Errorf("Missing mariadb packages. Unable to migrate from cf-mysql-release to pxc-release. In order to migrate from cf-mysql-release, both releases must be deployed on the same instance group.")
	}
	mariadbCmd := exec.Command("/var/vcap/packages/mariadb/bin/mysqld_safe", "--defaults-file=/var/vcap/jobs/mysql/config/my.cnf", "--wsrep-on=OFF", "--wsrep-desync=ON", "--wsrep-OSU-method=RSU", "--wsrep-provider='none'", "--skip-networking")
	err = mariadbCmd.Start()
	return err
}
