package start_manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-golang/lager"
)

const (
	Clustered      = "CLUSTERED"
	NeedsBootstrap = "NEEDS_BOOTSTRAP"
	SingleNode     = "SINGLE_NODE"

	BootstrapCommand = "bootstrap"
	JoinCommand      = "start"
)

type StartManager struct {
	osHelper                os_helper.OsHelper
	stateFileLocation       string
	mysqlClientPath         string
	jobIndex                int
	numberOfNodes           int
	dbSeedScriptPath        string
	showDatabasesScriptPath string
	clusterHealthChecker    cluster_health_checker.ClusterHealthChecker
	maxDatabaseSeedTries    int
	mariaDBHelper           mariadb_helper.DBHelper
	upgrader                upgrader.Upgrader
	logger                  lager.Logger
}

func New(
	osHelper os_helper.OsHelper,
	mariaDBHelper mariadb_helper.DBHelper,
	upgrader upgrader.Upgrader,
	stateFileLocation string,
	dbSeedScriptPath string,
	jobIndex int,
	numberOfNodes int,
	logger lager.Logger,
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker,
	maxDatabaseSeedTries int) *StartManager {
	return &StartManager{
		osHelper:             osHelper,
		stateFileLocation:    stateFileLocation,
		jobIndex:             jobIndex,
		numberOfNodes:        numberOfNodes,
		logger:               logger,
		dbSeedScriptPath:     dbSeedScriptPath,
		clusterHealthChecker: clusterHealthChecker,
		maxDatabaseSeedTries: maxDatabaseSeedTries,
		mariaDBHelper:        mariaDBHelper,
		upgrader:             upgrader,
	}
}

func (m *StartManager) Execute() (err error) {
	needsUpgrade, err := m.upgrader.NeedsUpgrade()
	if err != nil {
		m.logger.Info((fmt.Sprintf("Failed to determine upgrade status with error: %s", err.Error())))
		return
	}
	if needsUpgrade {
		err = m.upgrader.Upgrade()
		if err != nil {
			m.logger.Info((fmt.Sprintf("Failed to upgrade with error: %s", err.Error())))
			return
		}
	}

	// Single-node deploy always bootstraps new cluster
	if m.numberOfNodes == 1 {
		m.logger.Info("Single node deploy")
		err = m.bootstrapCluster(SingleNode)
		return
	}

	// If there is no state file, we must be a new deploy.
	if !m.osHelper.FileExists(m.stateFileLocation) {
		// In this case node 0 will bootstrap
		if m.jobIndex == 0 {
			m.logger.Info(fmt.Sprintf("state file does not exist, creating with contents: '%s'", Clustered))
			err = m.bootstrapCluster(Clustered)
			return
		} // Other nodes join existing cluster
		err = m.joinCluster()
		return
	}

	file_contents, _ := m.osHelper.ReadFile(m.stateFileLocation)
	state := strings.TrimSpace(file_contents)
	m.logger.Info(fmt.Sprintf("state file exists and contains: '%s'", state))
	switch state {
	case SingleNode:
		// Upgrading from a single-node cluster means we have to re-bootstrap
		err = m.bootstrapCluster(Clustered)
	case Clustered:
		err = m.joinCluster()
	case NeedsBootstrap:
		err = m.bootstrapCluster(Clustered)
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", state)
	}
	return
}

func (m *StartManager) bootstrapCluster(state string) (err error) {
	err = m.bootstrapNode()
	if err != nil {
		return
	}

	err = m.seedDatabases()
	if err != nil {
		return
	}

	m.logger.Info(fmt.Sprintf("writing file with contents: '%s'", state))
	m.osHelper.WriteStringToFile(m.stateFileLocation, state)
	return
}

func (m *StartManager) bootstrapNode() error {
	var command string

	// We do not condone bootstrapping if a cluster already exists and is healthy
	if m.clusterHealthChecker.HealthyCluster() {
		command = JoinCommand
	} else {
		command = BootstrapCommand
	}

	err := m.mariaDBHelper.StartMysqldInMode(command)
	if err != nil {
		return err
	}
	return nil
}

func (m *StartManager) joinCluster() (err error) {
	err = m.mariaDBHelper.StartMysqldInMode(JoinCommand)
	if err != nil {
		return err
	}

	// We should always seed databases even when joining an existing cluster,
	// as this encompasses the case where we're redeploying to an existing
	// cluster but with new databases to seed.
	err = m.seedDatabases()
	if err != nil {
		return
	}

	m.writeStringToFile(Clustered)
	return nil
}

func (m *StartManager) writeStringToFile(contents string) {
	m.logger.Info(fmt.Sprintf("updating file with contents: '%s'", contents))
	m.osHelper.WriteStringToFile(m.stateFileLocation, contents)
}

func (m *StartManager) seedDatabases() (err error) {
	var output string
	for numTries := 0; numTries < m.maxDatabaseSeedTries; numTries++ {
		output, err = m.osHelper.RunCommand("bash", m.dbSeedScriptPath)
		if err == nil {
			m.logger.Info("Seeding databases succeeded.")
			return
		}
		m.logger.Info(fmt.Sprintf("There was a problem seeding the database: '%s'", output))
		m.logger.Info("Retrying seeding script...")
		m.osHelper.Sleep(1 * time.Second)
	}

	m.logger.Info(fmt.Sprintf("Error seeding databases: '%s'\n'%s'", err.Error(), output))
	m.mariaDBHelper.StopStandaloneMysql()
	return
}
