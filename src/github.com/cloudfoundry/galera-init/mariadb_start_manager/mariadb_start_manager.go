package mariadb_start_manager

import (
	"fmt"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/galera_helper"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
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
	mariaDBHelper              mariadb_helper.DBHelper
	upgrader                   upgrader.Upgrader
}

func New(
	osHelper os_helper.OsHelper,
	mariaDBHelper mariadb_helper.DBHelper,
	upgrader upgrader.Upgrader,
	logFileLocation string,
	stateFileLocation string,
	mysqlDaemonPath string,
	username string,
	password string,
	dbSeedScriptPath string,
	jobIndex int,
	numberOfNodes int,
	loggingOn bool,
	upgradeScriptPath string,
	clusterReachabilityChecker galera_helper.ClusterReachabilityChecker,
	maxDatabaseSeedTries int) *MariaDBStartManager {
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
		upgrader:                   upgrader,
	}
}

func (m *MariaDBStartManager) Log(info string) {
	if m.loggingOn {
		fmt.Printf("%v ----- %v", time.Now().Local(), info)
	}
}

func (m *MariaDBStartManager) Execute() (err error) {
	needsUpgrade, err := m.upgrader.NeedsUpgrade()
	if err != nil {
		m.Log((fmt.Sprintf("Failed to determine upgrade status with error %s, exiting", err.Error())))
		return
	}
	if needsUpgrade {
		err = m.upgrader.Upgrade()
		if err != nil {
			m.Log((fmt.Sprintf("Failed to upgrade with error %s, exiting", err.Error())))
			return
		}
	}

	// Nodes > 0 always join an existing cluster
	if m.jobIndex != 0 {
		err = m.joinCluster()
		return
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
