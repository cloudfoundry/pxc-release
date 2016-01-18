package start_manager

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
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

	StartupPollingFrequencyInSeconds = 5
)

type StartManager interface {
	Execute() error
	GetMysqlCmd() (*exec.Cmd, error)
	Shutdown() error
}

type startManager struct {
	osHelper             os_helper.OsHelper
	config               config.StartManager
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker
	mariaDBHelper        mariadb_helper.DBHelper
	upgrader             upgrader.Upgrader
	logger               lager.Logger
	mysqlCmd             *exec.Cmd
	monitorJoin          bool
}

func New(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	mariaDBHelper mariadb_helper.DBHelper,
	upgrader upgrader.Upgrader,
	logger lager.Logger,
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker,
	monitorJoin bool) StartManager {
	return &startManager{
		osHelper:             osHelper,
		config:               config,
		logger:               logger,
		clusterHealthChecker: clusterHealthChecker,
		mariaDBHelper:        mariaDBHelper,
		upgrader:             upgrader,
		monitorJoin:          monitorJoin,
	}
}

func (m *startManager) Execute() error {

	if m.mariaDBHelper.IsProcessRunning() {
		m.logger.Info("MySQL process is already running, shutting down before continuing")
		err := m.Shutdown()
		if err != nil {
			m.logger.Error("Failed to shutdown mysql process", err)
			return err
		}
	}

	needsUpgrade, err := m.upgrader.NeedsUpgrade()
	if err != nil {
		m.logger.Info("Failed to determine upgrade status with error", lager.Data{"err": err.Error()})
		return err
	}
	if needsUpgrade {
		err = m.upgrader.Upgrade()
		if err != nil {
			m.logger.Info("Failed to upgrade", lager.Data{"err": err.Error()})
			return err
		}
	}

	m.logger.Info("Determining bootstrap procedure", lager.Data{
		"ClusterIps": m.config.ClusterIps,
		"MyIP":       m.config.MyIP,
	})

	currentState, err := m.getCurrentNodeState()
	if err != nil {
		return err
	}

	var newNodeState string
	switch currentState {
	case SingleNode:
		err = m.bootstrapSingleNode()
		newNodeState = SingleNode
	case NeedsBootstrap:
		err = m.bootstrapCluster()
		newNodeState = Clustered
	case Clustered:
		err = m.joinCluster()
		newNodeState = Clustered
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", currentState)
	}
	if err != nil {
		return err
	}

	err = m.waitForDatabaseToAcceptConnections()
	if err != nil {
		return err
	}

	err = m.seedDatabases()
	if err != nil {
		return err
	}

	m.writeStringToFile(newNodeState)

	return err
}

func (m *startManager) getCurrentNodeState() (string, error) {

	// Single-node deploy always requires bootstraping of new cluster
	if len(m.config.ClusterIps) == 1 {
		return SingleNode, nil
	}

	if m.firstTimeDeploy() {
		if m.config.MyIP == m.config.ClusterIps[0] {
			return NeedsBootstrap, nil
		}

		return Clustered, nil
	}

	// If we are not a first time deploy we must already have a state file
	state, err := m.readStateFromFile()
	if err != nil {
		m.logger.Info("state file could not be read", lager.Data{"err": err.Error()})
		return "", err
	}

	if state == SingleNode && len(m.config.ClusterIps) > 1 {
		// Upgrading from a single-node cluster means we have to re-bootstrap
		return NeedsBootstrap, nil
	}

	return state, nil
}

func (m *startManager) maxDatabaseSeedTries() int {
	return m.config.DatabaseStartupTimeout / StartupPollingFrequencyInSeconds
}

func (m *startManager) readStateFromFile() (string, error) {
	state, err := m.osHelper.ReadFile(m.config.StateFileLocation)
	if err != nil {
		return "", err
	}
	state = strings.TrimSpace(state)
	m.logger.Info(fmt.Sprintf("state file exists and contains: '%s'", state))
	return state, nil
}

func (m *startManager) firstTimeDeploy() bool {
	return !m.osHelper.FileExists(m.config.StateFileLocation)
}

func (m *startManager) GetMysqlCmd() (*exec.Cmd, error) {
	if m.mysqlCmd != nil {
		return m.mysqlCmd, nil
	}
	return nil, errors.New("Mysql has not been started")
}

func (m *startManager) Shutdown() error {
	m.logger.Info("Shutting down MariaDB")
	return m.mariaDBHelper.StopMysql()
}

func (m *startManager) bootstrapSingleNode() error {

	m.logger.Info("Bootstrapping a single node cluster")
	cmd, err := m.mariaDBHelper.StartMysqlInBootstrap()
	if err != nil {
		return err
	}
	m.mysqlCmd = cmd

	return nil
}

func (m *startManager) bootstrapCluster() error {

	m.logger.Info("Bootstrapping a multi-node cluster")
	var cmd *exec.Cmd
	var err error
	// We do not condone bootstrapping if a cluster already exists and is healthy
	if m.clusterHealthChecker.HealthyCluster() {
		cmd, err = m.mariaDBHelper.StartMysqlInJoin()
	} else {
		cmd, err = m.mariaDBHelper.StartMysqlInBootstrap()
	}

	if err != nil {
		return err
	}

	m.mysqlCmd = cmd

	return nil
}

func (m *startManager) joinCluster() (err error) {

	m.logger.Info("Joining a multi-node cluster")
	if !m.monitorJoin {
		cmd, err := m.mariaDBHelper.StartMysqlInJoin()

		if err != nil {
			return err
		}

		m.mysqlCmd = cmd

		return nil
	} else {
		err := m.mariaDBHelper.StartMysqlInJoinMonitored()
		if err != nil {
			return err
		}

		return nil

	}
}

func (m *startManager) writeStringToFile(contents string) {
	m.logger.Info(fmt.Sprintf("updating file with contents: '%s'", contents))
	m.osHelper.WriteStringToFile(m.config.StateFileLocation, contents)
}

func (m *startManager) waitForDatabaseToAcceptConnections() error {
	m.logger.Info(fmt.Sprintf("Attempting to reach database. Timeout is %d seconds", m.config.DatabaseStartupTimeout))
	for numTries := 0; numTries < m.maxDatabaseSeedTries(); numTries++ {
		if m.mariaDBHelper.IsDatabaseReachable() {
			m.logger.Info(fmt.Sprintf("Database became reachable after %d seconds", numTries*StartupPollingFrequencyInSeconds))
			return nil
		}
		m.logger.Info("Database not reachable, retrying...")
		m.osHelper.Sleep(StartupPollingFrequencyInSeconds * time.Second)
	}

	err := fmt.Errorf("Timeout: Database not reachable after %d seconds", m.config.DatabaseStartupTimeout)
	m.logger.Info(fmt.Sprintf("Error reachable databases: '%s'", err.Error()))
	return err
}

func (m *startManager) seedDatabases() error {
	err := m.mariaDBHelper.Seed()
	if err != nil {
		m.logger.Info(fmt.Sprintf("There was a problem seeding the database: '%s'", err.Error()))
		return err
	}

	m.logger.Info("Seeding databases succeeded.")
	return nil
}
