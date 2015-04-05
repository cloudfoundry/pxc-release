package main

import (
	"flag"
	"os"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
)

var (
	flags                   = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	packageVersionFile      = flags.String("packagingVersionFile", "/var/vcap/packages/mariadb/VERSION", "Specifies the location of the file containing the MySQL version as deployed")
	lastUpgradedVersionFile = flags.String("lastUpgradedVersionFile", "/var/vcap/store/mysql/mysql_upgrade_info", "Specifies the location of the file MySQL upgrade writes.")

	logFileLocation = flags.String("logFile", "", "Specifies the location of the log file mysql sends logs to")

	mysqlDaemonPath  = flags.String("mysqlDaemon", "", "Specifies the location of the script that starts and stops mysql using mysqld_safe and mysql.server")
	mysqlClientPath  = flags.String("mysqlClient", "", "Specifies the location of the mysql client executable")
	mysqlUpgradePath = flags.String("mysqlUpgradePath", "/var/vcap/packages/mariadb/bin/mysql_upgrade", "Specifies the location of the script that performs the MySQL upgrade")

	dbSeedScriptPath = flags.String("dbSeedScript", "", "Specifies the location of the script that seeds the server with databases")

	stateFileLocation = flags.String("stateFile", "", "Specifies the location to store the statefile for MySQL boot")

	mysqlUser     = flags.String("mysqlUser", "root", "Specifies the user name for MySQL")
	mysqlPassword = flags.String("mysqlPassword", "", "Specifies the password for connecting to MySQL")

	jobIndex             = flags.Int("jobIndex", 1, "Specifies the job index of the MySQL node")
	numberOfNodes        = flags.Int("numberOfNodes", 3, "Number of nodes deployed in the galera cluster")
	clusterIps           = flags.String("clusterIps", "", "Comma-delimited list of IPs in the galera cluster")
	maxDatabaseSeedTries = flags.Int("maxDatabaseSeedTries", 1, "How many times to attempt database seeding before it fails")
)

func main() {
	cf_lager.AddFlags(flags)
	flags.Parse(os.Args[1:])

	logger, _ := cf_lager.New("mariadb_ctrl")
	osHelper := os_helper.NewImpl()

	mariaDBHelper := mariadb_helper.NewMariaDBHelper(
		osHelper,
		*mysqlDaemonPath,
		*mysqlClientPath,
		*logFileLocation,
		logger,
		*mysqlUpgradePath,
		*mysqlUser,
		*mysqlPassword,
	)

	upgrader := upgrader.NewUpgrader(
		*packageVersionFile,
		*lastUpgradedVersionFile,
		osHelper,
		logger,
		mariaDBHelper,
	)

	galeraHelper := cluster_health_checker.NewClusterHealthChecker(*clusterIps, logger)

	mgr := start_manager.New(
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
