package main

import (
	"flag"
	"os"

	"github.com/cloudfoundry/mariadb_ctrl/galera_helper"
	. "github.com/cloudfoundry/mariadb_ctrl/logger"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	manager "github.com/cloudfoundry/mariadb_ctrl/mariadb_start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
)

var (
	PACKAGE_VERSION_FILE       = "/var/vcap/packages/mariadb/VERSION"
	LAST_UPGRADED_VERSION_FILE = "/var/vcap/store/mysql/mysql_upgrade_info"
	LOGGING_ON                 = true

	logFileLocation = flag.String(
		"logFile",
		"",
		"Specifies the location of the log file mysql sends logs to",
	)

	mysqlDaemonPath = flag.String(
		"mysqlDaemon",
		"",
		"Specifies the location of the script that starts and stops mysql using mysqld_safe and mysql.server",
	)

	mysqlClientPath = flag.String(
		"mysqlClient",
		"",
		"Specifies the location of the mysql client executable",
	)

	dbSeedScriptPath = flag.String(
		"dbSeedScript",
		"",
		"Specifies the location of the script that seeds the server with databases",
	)

	upgradeScriptPath = flag.String(
		"upgradeScriptPath",
		"",
		"Specifies the location of the script that performs the MySQL upgrade",
	)

	showDatabasesScriptPath = flag.String(
		"showDatabasesScriptPath",
		"",
		"Specifies the location of the script that displays the MySQL databases",
	)

	stateFileLocation = flag.String(
		"stateFile",
		"",
		"Specifies the location to store the statefile for MySQL boot",
	)

	mysqlUser = flag.String(
		"mysqlUser",
		"root",
		"Specifies the user name for MySQL",
	)

	mysqlPassword = flag.String(
		"mysqlPassword",
		"",
		"Specifies the password for connecting to MySQL",
	)

	jobIndex = flag.Int(
		"jobIndex",
		1,
		"Specifies the job index of the MySQL node",
	)

	numberOfNodes = flag.Int(
		"numberOfNodes",
		3,
		"Number of nodes deployed in the galera cluster",
	)

	clusterIps = flag.String(
		"clusterIps",
		"",
		"Comma-delimited list of IPs in the galera cluster",
	)

	maxDatabaseSeedTries = flag.Int(
		"maxDatabaseSeedTries",
		1,
		"How many times to attempt database seeding before it fails",
	)
)

func main() {
	flag.Parse()

	logger := NewStdOutLogger(LOGGING_ON)
	osHelper := os_helper.NewImpl()

	mariaDBHelper := mariadb_helper.NewMariaDBHelper(
		osHelper,
		*mysqlDaemonPath,
		*mysqlClientPath,
		*logFileLocation,
		logger,
		*upgradeScriptPath,
		*showDatabasesScriptPath,
		*mysqlUser,
		*mysqlPassword,
	)

	upgrader := upgrader.NewImpl(
		*upgradeScriptPath,
		*mysqlDaemonPath,
		PACKAGE_VERSION_FILE,
		LAST_UPGRADED_VERSION_FILE,
		osHelper,
		logger,
		mariaDBHelper,
	)

	galeraHelper := galera_helper.NewClusterReachabilityChecker(*clusterIps, logger)

	mgr := manager.New(
		osHelper,
		mariaDBHelper,
		upgrader,
		*logFileLocation,
		*stateFileLocation,
		*mysqlUser,
		*mysqlPassword,
		*dbSeedScriptPath,
		*jobIndex,
		*numberOfNodes,
		logger,
		*upgradeScriptPath,
		galeraHelper,
		*maxDatabaseSeedTries,
	)

	err := mgr.Execute()
	if err != nil {
		logger.Log("Execution exited with an error")
		os.Exit(1)
	}
}
