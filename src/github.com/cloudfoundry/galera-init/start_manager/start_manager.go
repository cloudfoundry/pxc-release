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
)

type StartManager struct {
	osHelper             os_helper.OsHelper
	config               config.StartManager
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker
	mariaDBHelper        mariadb_helper.DBHelper
	upgrader             upgrader.Upgrader
	logger               lager.Logger
	mysqlCmd             *exec.Cmd
}

func New(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	mariaDBHelper mariadb_helper.DBHelper,
	upgrader upgrader.Upgrader,
	logger lager.Logger,
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker) *StartManager {
	return &StartManager{
		osHelper:             osHelper,
		config:               config,
		logger:               logger,
		clusterHealthChecker: clusterHealthChecker,
		mariaDBHelper:        mariaDBHelper,
		upgrader:             upgrader,
	}
}

func (m *StartManager) Execute() error {
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

	// Single-node deploy always requires bootstraping of new cluster
	if len(m.config.ClusterIps) == 1 {
		m.logger.Info("Single node deploy")
		err = m.bootstrapCluster(SingleNode)
		return err
	}

	if m.firstTimeDeploy() {
		if m.config.MyIP == m.config.ClusterIps[0] {
			m.logger.Info(fmt.Sprintf("state file does not exist, creating with contents: '%s'", Clustered))
			err = m.bootstrapCluster(Clustered)
			return err
		}

		err = m.joinCluster()
		return err
	}

	// If we are not a first time deploy we must already have a state file
	state, err := m.readStateFromFile()
	if err != nil {
		m.logger.Info("state file could not be read", lager.Data{"err": err.Error()})
		return err
	}
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
	return err
}

func (m *StartManager) readStateFromFile() (string, error) {
	state, err := m.osHelper.ReadFile(m.config.StateFileLocation)
	if err != nil {
		return "", err
	}
	state = strings.TrimSpace(state)
	m.logger.Info(fmt.Sprintf("state file exists and contains: '%s'", state))
	return state, nil
}

func (m *StartManager) firstTimeDeploy() bool {
	return !m.osHelper.FileExists(m.config.StateFileLocation)
}

func (m *StartManager) GetMysqlCmd() (*exec.Cmd, error) {
	if m.mysqlCmd != nil {
		return m.mysqlCmd, nil
	}
	return nil, errors.New("Mysql has not been started")
}

func (m *StartManager) Shutdown() error {
	m.logger.Info("Shutting down MariaDB")
	return m.mariaDBHelper.StopMysql()
}

func (m *StartManager) bootstrapCluster(state string) (err error) {

	m.logger.Info("Bootstrapping cluster")

	if state == SingleNode {
		err = m.bootstrapSingleNode()
	} else {
		err = m.bootstrapClusterNode()
	}

	if err != nil {
		return
	}

	err = m.seedDatabases()
	if err != nil {
		return
	}

	m.logger.Info(fmt.Sprintf("writing file with contents: '%s'", state))
	m.osHelper.WriteStringToFile(m.config.StateFileLocation, state)
	return
}

func (m *StartManager) bootstrapSingleNode() error {
	cmd, err := m.mariaDBHelper.StartMysqlInBootstrap()
	if err != nil {
		return err
	}
	m.mysqlCmd = cmd

	return nil
}

func (m *StartManager) bootstrapClusterNode() error {

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

func (m *StartManager) joinCluster() (err error) {
	cmd, err := m.mariaDBHelper.StartMysqlInJoin()

	if err != nil {
		return err
	}

	m.mysqlCmd = cmd

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
	m.osHelper.WriteStringToFile(m.config.StateFileLocation, contents)
}

func (m *StartManager) seedDatabases() error {
	for numTries := 0; numTries < m.config.MaxDatabaseSeedTries; numTries++ {
		if !m.mariaDBHelper.IsDatabaseReachable() {
			m.logger.Info("Database not reachable, retrying...")
			m.osHelper.Sleep(5 * time.Second)
			continue
		}

		err := m.mariaDBHelper.Seed()
		if err != nil {
			m.logger.Info(fmt.Sprintf("There was a problem seeding the database: '%s'", err.Error()))
			return err
		}

		m.logger.Info("Seeding databases succeeded.")
		return nil
	}

	err := fmt.Errorf("Database not reachable after %d attempts", m.config.MaxDatabaseSeedTries)
	m.logger.Info(fmt.Sprintf("Error reachable databases: '%s'", err.Error()))
	return err
}
