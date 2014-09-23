package mariadb_start_manager

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/galera_helper"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
)

const (
	CLUSTERED   = "CLUSTERED"
	SINGLE_NODE = "SINGLE_NODE"

	BOOTSTRAP_COMMAND = "bootstrap"
	JOIN_COMMAND      = "start"
)

type MariaDBStartManager struct {
	osHelper                   os_helper.OsHelper
	logFileLocation            string
	stateFileLocation          string
	mysqlDaemonPath            string
	mysqlClientPath            string
	username                   string
	password                   string
	jobIndex                   int
	numberOfNodes              int
	loggingOn                  bool
	dbSeedScriptPath           string
	upgradeScriptPath          string
	showDatabasesScriptPath    string
	ClusterReachabilityChecker galera_helper.ClusterReachabilityChecker
	maxDatabaseSeedTries       int
	mariaDBHelper              *mariadb_helper.MariaDBHelper
}

func New(osHelper os_helper.OsHelper,
	logFileLocation string,
	stateFileLocation string,
	mysqlDaemonPath string,
	mysqlClientPath string,
	username string,
	password string,
	dbSeedScriptPath string,
	jobIndex int,
	numberOfNodes int,
	loggingOn bool,
	upgradeScriptPath string,
	showDatabasesScriptPath string,
	clusterReachabilityChecker galera_helper.ClusterReachabilityChecker,
	maxDatabaseSeedTries int) *MariaDBStartManager {
	mariaDBHelper := mariadb_helper.NewMariaDBHelper(
		osHelper,
		mysqlDaemonPath,
		mysqlClientPath,
		logFileLocation,
		loggingOn,
		upgradeScriptPath,
		showDatabasesScriptPath,
		username,
		password,
	)
	return &MariaDBStartManager{
		osHelper:                   osHelper,
		logFileLocation:            logFileLocation,
		stateFileLocation:          stateFileLocation,
		username:                   username,
		password:                   password,
		jobIndex:                   jobIndex,
		mysqlDaemonPath:            mysqlDaemonPath,
		numberOfNodes:              numberOfNodes,
		loggingOn:                  loggingOn,
		dbSeedScriptPath:           dbSeedScriptPath,
		upgradeScriptPath:          upgradeScriptPath,
		ClusterReachabilityChecker: clusterReachabilityChecker,
		maxDatabaseSeedTries:       maxDatabaseSeedTries,
		mariaDBHelper:              mariaDBHelper,
	}
}

func (m *MariaDBStartManager) Log(info string) {
	if m.loggingOn {
		fmt.Printf("%v ----- %v", time.Now().Local(), info)
	}
}

func (m *MariaDBStartManager) Execute() (err error) {
	// Nodes > 0 always join an existing cluster
	if m.jobIndex != 0 {
		err := m.joinCluster()
		return err
	}

	//single-node deploy
	if m.numberOfNodes == 1 {
		m.Log("Single node deploy\n")
		err = m.bootstrapCluster(SINGLE_NODE)
		return
	}

	//MULTI-NODE DEPLOYMENTS BELOW

	//intial deploy, state file does not exist
	if !m.osHelper.FileExists(m.stateFileLocation) {
		m.Log(fmt.Sprintf("state file does not exist, creating with contents: '%s'\n", CLUSTERED))
		err = m.bootstrapCluster(CLUSTERED)
		return
	}

	//state file exists
	orig_contents, _ := m.osHelper.ReadFile(m.stateFileLocation)
	m.Log(fmt.Sprintf("state file exists and contains: '%s'\n", orig_contents))

	//scaling up from a single node cluster
	if orig_contents == SINGLE_NODE {
		err = m.bootstrapCluster(CLUSTERED)
		return
	}

	err = m.node0JoinCluster()
	return
}

func (m *MariaDBStartManager) bootstrapCluster(state string) (err error) {
	err = m.bootstrapNode()
	if err != nil {
		return
	}

	err = m.seedDatabases()
	if err != nil {
		return
	}

	err = m.upgradeAndRestartIfNecessary(BOOTSTRAP_COMMAND)
	if err != nil {
		return
	}

	m.Log(fmt.Sprintf("writing file with contents: '%s'\n", state))
	m.osHelper.WriteStringToFile(m.stateFileLocation, state)
	return
}

func (m *MariaDBStartManager) bootstrapNode() error {
	var command string

	if m.ClusterReachabilityChecker.AnyNodesReachable() {
		command = JOIN_COMMAND
	} else {
		command = BOOTSTRAP_COMMAND
	}
	err := m.mariaDBHelper.StartMysqldInMode(command)
	if err != nil {
		return err
	}
	return nil
}

func (m *MariaDBStartManager) joinCluster() (err error) {
	err = m.mariaDBHelper.StartMysqldInMode(JOIN_COMMAND)
	if err != nil {
		return err
	}

	err = m.upgradeAndRestartIfNecessary(JOIN_COMMAND)
	if err != nil {
		return
	}

	m.writeStringToFile(CLUSTERED)
	return nil
}

func (m *MariaDBStartManager) writeStringToFile(contents string) {
	m.Log(fmt.Sprintf("updating file with contents: '%s'\n", contents))
	m.osHelper.WriteStringToFile(m.stateFileLocation, contents)
}

func (m *MariaDBStartManager) node0JoinCluster() (err error) {
	err = m.mariaDBHelper.StartMysqldInMode(JOIN_COMMAND)
	if err != nil {
		return err
	}

	err = m.seedDatabases()
	if err != nil {
		return
	}

	err = m.upgradeAndRestartIfNecessary(JOIN_COMMAND)
	if err != nil {
		return
	}

	m.writeStringToFile(CLUSTERED)
	return nil
}

func (m *MariaDBStartManager) seedDatabases() (err error) {
	var output string
	for numTries := 0; numTries < m.maxDatabaseSeedTries; numTries++ {
		output, err = m.osHelper.RunCommand("bash", m.dbSeedScriptPath)
		if err == nil {
			m.Log("Seeding databases succeeded.\n")
			return
		} else {
			m.Log(fmt.Sprintf("There was a problem seeding the database: 's%'\n", output))
			m.Log("Retrying seeding script...\n")
			m.osHelper.Sleep(1 * time.Second)
		}
	}

	m.Log(fmt.Sprintf("Error seeding databases: '%s'\n'%s'\n", err.Error(), output))
	m.mariaDBHelper.StopMysqld()
	return
}

func (m *MariaDBStartManager) upgradeAndRestartIfNecessary(command string) (err error) {
	m.Log("waiting for database to be ready...\n")
	for i := 0; i < 600; i++ {
		if m.mariaDBHelper.IsDatabaseReachable() {
			break
		}
		m.Log("Database not ready, sleeping 5 seconds...\n")
		m.osHelper.Sleep(5 * time.Second)
	}

	m.Log("performing upgrade\n")

	upgradeOutput, upgradeErr := m.mariaDBHelper.Upgrade()

	if m.requiresRestart(upgradeOutput, upgradeErr) {
		err = m.mariaDBHelper.StopMysqld()
		if err != nil {
			m.Log(fmt.Sprintf("Error: %s\n", err.Error()))
			return err
		}

		if command == BOOTSTRAP_COMMAND && m.ClusterReachabilityChecker.AnyNodesReachable() {
			command = JOIN_COMMAND
		}

		err = m.mariaDBHelper.StartMysqldInMode(command)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MariaDBStartManager) requiresRestart(output string, err error) bool {
	// No error indicates that the upgrade script performed an upgrade.
	if err == nil {
		m.Log("upgrade sucessful - restart required\n")
		return true
	}
	m.Log(fmt.Sprintf("upgrade output: %s\n", output))

	//known error messages where a restart should not occur, do not remove from
	acceptableErrorsCompiled, _ := regexp.Compile("already upgraded|Unknown command|WSREP has not yet prepared node")
	if acceptableErrorsCompiled.MatchString(output) {
		m.Log("output string matches acceptable errors - skip restart\n")
		return false
	} else {
		m.Log("output string does not match acceptable errors - restart required\n")
		return true
	}
}
