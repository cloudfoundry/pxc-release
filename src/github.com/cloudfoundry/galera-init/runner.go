package main

import (
	"flag"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	manager "github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-golang/lager"
)

var (
	packageVersionFile      = flag.String("packagingVersionFile", "/var/vcap/packages/mariadb/VERSION", "Specifies the location of the file containing the MySQL version as deployed")
	lastUpgradedVersionFile = flag.String("lastUpgradedVersionFile", "/var/vcap/store/mysql/mysql_upgrade_info", "Specifies the location of the file MySQL upgrade writes.")

	loggingOn       = flag.Bool("loggingOn", true, "Specifies whether logging is enabled")
	logFileLocation = flag.String("logFile", "", "Specifies the location of the log file mysql sends logs to")

	mysqlDaemonPath = flag.String("mysqlDaemon", "", "Specifies the location of the script that starts and stops mysql using mysqld_safe and mysql.server")
	mysqlClientPath = flag.String("mysqlClient", "", "Specifies the location of the mysql client executable")

	dbSeedScriptPath        = flag.String("dbSeedScript", "", "Specifies the location of the script that seeds the server with databases")
	upgradeScriptPath       = flag.String("upgradeScriptPath", "", "Specifies the location of the script that performs the MySQL upgrade")
	showDatabasesScriptPath = flag.String("showDatabasesScriptPath", "", "Specifies the location of the script that displays the MySQL databases")

	stateFileLocation = flag.String("stateFile", "", "Specifies the location to store the statefile for MySQL boot")

	mysqlUser     = flag.String("mysqlUser", "root", "Specifies the user name for MySQL")
	mysqlPassword = flag.String("mysqlPassword", "", "Specifies the password for connecting to MySQL")

	jobIndex             = flag.Int("jobIndex", 1, "Specifies the job index of the MySQL node")
	numberOfNodes        = flag.Int("numberOfNodes", 3, "Number of nodes deployed in the galera cluster")
	clusterIps           = flag.String("clusterIps", "", "Comma-delimited list of IPs in the galera cluster")
	maxDatabaseSeedTries = flag.Int("maxDatabaseSeedTries", 1, "How many times to attempt database seeding before it fails")
)

func main() {
	flag.Parse()

	logger := lager.NewLogger("mariadb_ctrl")
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
		*packageVersionFile,
		*lastUpgradedVersionFile,
		osHelper,
		logger,
		mariaDBHelper,
	)

	galeraHelper := cluster_health_checker.NewClusterHealthChecker(*clusterIps, logger)

	mgr := manager.New(
		osHelper,
		mariaDBHelper,
		upgrader,
		*stateFileLocation,
		*dbSeedScriptPath,
		*jobIndex,
		*numberOfNodes,
		logger,
		galeraHelper,
		*maxDatabaseSeedTries,
	)

	err := mgr.Execute()
	if err != nil {
		logger.Fatal("Execution exited with an error", err)
	}
}
