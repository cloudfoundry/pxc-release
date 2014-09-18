package main

import (
	"flag"
	"os"

	"github.com/cloudfoundry/mariadb_ctrl/galera_helper"
	manager "github.com/cloudfoundry/mariadb_ctrl/mariadb_start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
)

var logFileLocation = flag.String(
	"logFile",
	"",
	"Specifies the location of the log file mysql sends logs to",
)

var mysqlDaemonPath = flag.String(
	"mysqlDaemon",
	"",
	"Specifies the location of the script that starts and stops mysql using mysqld_safe and mysql.server",
)

var dbSeedScriptPath = flag.String(
	"dbSeedScript",
	"",
	"Specifies the location of the script that seeds the server with databases",
)

var upgradeScriptPath = flag.String(
	"upgradeScriptPath",
	"",
	"Specifies the location of the script that performs the MySQL upgrade",
)

var stateFileLocation = flag.String(
	"stateFile",
	"",
	"Specifies the location to store the statefile for MySQL boot",
)

var mysqlUser = flag.String(
	"mysqlUser",
	"root",
	"Specifies the user name for MySQL",
)

var mysqlPassword = flag.String(
	"mysqlPassword",
	"",
	"Specifies the password for connecting to MySQL",
)

var jobIndex = flag.Int(
	"jobIndex",
	1,
	"Specifies the job index of the MySQL node",
)

var numberOfNodes = flag.Int(
	"numberOfNodes",
	3,
	"Number of nodes deployed in the galera cluster",
)

var clusterIps = flag.String(
	"clusterIps",
	"",
	"Comma-delimited list of IPs in the galera cluster",
)

var maxDatabaseSeedTries = flag.Int(
	"maxDatabaseSeedTries",
	1,
	"How many times to attempt database seeding before it fails",
)

func main() {
	flag.Parse()

	mgr := manager.New(
		os_helper.NewImpl(),
		*logFileLocation,
		*stateFileLocation,
		*mysqlDaemonPath,
		*mysqlUser,
		*mysqlPassword,
		*dbSeedScriptPath,
		*jobIndex,
		*numberOfNodes,
		true,
		*upgradeScriptPath,
		nil,
		*maxDatabaseSeedTries,
	)
	mgr.ClusterReachabilityChecker = galera_helper.NewClusterReachabilityChecker(*clusterIps, mgr)
	err := mgr.Execute()
	if err != nil {
		mgr.Log("Execution exited with an error\n")
		os.Exit(1)
	}
}
