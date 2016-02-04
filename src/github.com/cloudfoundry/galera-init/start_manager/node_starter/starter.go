package node_starter

import (
	"errors"
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
	GetMysqlCmd() (*exec.Cmd, error)
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

func (s *starter) StartNodeFromState(state string) (string, error) {
	var newNodeState string
	var err error
	var bootStrapNode bool

	switch state {
	case SingleNode:
		err = s.bootstrapNode()
		newNodeState = SingleNode
		bootStrapNode = true
	case NeedsBootstrap:
		if s.clusterHealthChecker.HealthyCluster() {
			err = s.startNodeAsJoiner()
			bootStrapNode = false
		} else {
			err = s.bootstrapNode()
			bootStrapNode = true
		}
		newNodeState = Clustered
	case Clustered:
		err = s.joinCluster()
		newNodeState = Clustered
		bootStrapNode = false
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

	if bootStrapNode {
		err = s.seedDatabases()
		if err != nil {
			return "", err
		}

		err = s.createReadOnlyUser()
		if err != nil {
			return "", err
		}
	}

	return newNodeState, nil
}

func (s *starter) GetMysqlCmd() (*exec.Cmd, error) {
	if s.mysqlCmd != nil {
		return s.mysqlCmd, nil
	}
	return nil, errors.New("Mysql has not been started")
}

func (s *starter) bootstrapNode() error {
	s.logger.Info("Bootstrapping node")
	cmd, err := s.mariaDBHelper.StartMysqlInBootstrap()
	if err != nil {
		return err
	}
	s.mysqlCmd = cmd

	return nil
}

func (s *starter) startNodeAsJoiner() error {
	s.logger.Info("Joining an existing cluster")
	cmd, err := s.mariaDBHelper.StartMysqlInJoin()
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
	err := s.mariaDBHelper.ManageReadOnlyUser()
	if err != nil {
		s.logger.Info(fmt.Sprintf("There was a problem creating the read only user: '%s'", err.Error()))
		return err
	}

	s.logger.Info("Creating read only user succeeded.")
	return nil
}
