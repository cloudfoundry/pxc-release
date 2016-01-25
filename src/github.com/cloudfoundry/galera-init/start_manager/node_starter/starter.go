package node_starter

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/pivotal-golang/lager"
)

const (
	Clustered                        = "CLUSTERED"
	NeedsBootstrap                   = "NEEDS_BOOTSTRAP"
	SingleNode                       = "SINGLE_NODE"
	StartupPollingFrequencyInSeconds = 5
)

//go:generate counterfeiter . Starter

type Starter interface {
	StartNodeFromState(string) (string, error)
}

type starter struct {
	mariaDBHelper        mariadb_helper.DBHelper
	osHelper             os_helper.OsHelper
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker
	config               config.StartManager
	logger               lager.Logger
	mysqlCmd             *exec.Cmd
}

func New(
	mariaDBHelper mariadb_helper.DBHelper,
	osHelper os_helper.OsHelper,
	config config.StartManager,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
) Starter {
	return &starter{
		mariaDBHelper:        mariaDBHelper,
		osHelper:             osHelper,
		config:               config,
		logger:               logger,
		clusterHealthChecker: healthChecker,
	}
}

func (s starter) StartNodeFromState(state string) (string, error) {
	var newNodeState string
	var err error

	switch state {
	case SingleNode:
		err = s.bootstrapSingleNode()
		newNodeState = SingleNode
	case NeedsBootstrap:
		err = s.bootstrapCluster()
		newNodeState = Clustered
	case Clustered:
		err = s.joinCluster()
		newNodeState = Clustered
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", state)
	}
	if err != nil {
		return "", err
	}

	err = s.waitForDatabaseToAcceptConnections()
	if err != nil {
		return "", err
	}

	err = s.seedDatabases()
	if err != nil {
		return "", err
	}

	err = s.createReadOnlyUser()
	if err != nil {
		return "", err
	}

	return newNodeState, nil
}

func (s *starter) bootstrapSingleNode() error {
	s.logger.Info("Bootstrapping a single node cluster")
	cmd, err := s.mariaDBHelper.StartMysqlInBootstrap()
	if err != nil {
		return err
	}

	s.mysqlCmd = cmd

	return nil
}

func (s *starter) bootstrapCluster() error {
	s.logger.Info("Bootstrapping a multi-node cluster")
	var cmd *exec.Cmd
	var err error
	// We do not condone bootstrapping if a cluster already exists and is healthy
	if s.clusterHealthChecker.HealthyCluster() {
		cmd, err = s.mariaDBHelper.StartMysqlInJoin()
	} else {
		cmd, err = s.mariaDBHelper.StartMysqlInBootstrap()
	}

	if err != nil {
		return err
	}

	s.mysqlCmd = cmd

	return nil
}

func (s *starter) joinCluster() (err error) {
	s.logger.Info("Joining a multi-node cluster")
	cmd, err := s.mariaDBHelper.StartMysqlInJoin()

	if err != nil {
		return err
	}

	s.mysqlCmd = cmd

	return nil
}

func (s *starter) maxDatabaseSeedTries() int {
	return s.config.DatabaseStartupTimeout / StartupPollingFrequencyInSeconds
}

func (s *starter) waitForDatabaseToAcceptConnections() error {
	s.logger.Info(fmt.Sprintf("Attempting to reach database. Timeout is %d seconds", s.config.DatabaseStartupTimeout))
	for numTries := 0; numTries < s.maxDatabaseSeedTries(); numTries++ {
		if s.mariaDBHelper.IsDatabaseReachable() {
			s.logger.Info(fmt.Sprintf("Database became reachable after %d seconds", numTries*StartupPollingFrequencyInSeconds))
			return nil
		}
		s.logger.Info("Database not reachable, retrying...")
		s.osHelper.Sleep(StartupPollingFrequencyInSeconds * time.Second)
	}

	err := fmt.Errorf("Timeout: Database not reachable after %d seconds", s.config.DatabaseStartupTimeout)
	s.logger.Info(fmt.Sprintf("Error reachable databases: '%s'", err.Error()))
	return err
}

func (s *starter) seedDatabases() error {
	err := s.mariaDBHelper.Seed()
	if err != nil {
		s.logger.Info(fmt.Sprintf("There was a problem seeding the database: '%s'", err.Error()))
		return err
	}

	s.logger.Info("Seeding databases succeeded.")
	return nil
}

func (s *starter) createReadOnlyUser() error {
	err := s.mariaDBHelper.CreateReadOnlyUser()
	if err != nil {
		s.logger.Info(fmt.Sprintf("There was a problem creating the read only user: '%s'", err.Error()))
		return err
	}

	s.logger.Info("Creating read only user succeeded.")
	return nil
}
