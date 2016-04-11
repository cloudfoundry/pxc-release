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

type prestarter struct {
	mariaDBHelper        mariadb_helper.DBHelper
	osHelper             os_helper.OsHelper
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker
	config               config.StartManager
	logger               lager.Logger
	mysqlCmd             *exec.Cmd
	finalState           string
}

func NewPreStarter(
	mariaDBHelper mariadb_helper.DBHelper,
	osHelper os_helper.OsHelper,
	config config.StartManager,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
) Starter {
	return &prestarter{
		mariaDBHelper:        mariaDBHelper,
		osHelper:             osHelper,
		config:               config,
		logger:               logger,
		clusterHealthChecker: healthChecker,
		finalState:           "",
	}
}

func (s *prestarter) StartNodeFromState(state string) (string, error) {
	var err error
	var newNodeState string

	switch state {
	case SingleNode:
		newNodeState = SingleNode
	case NeedsBootstrap:
		if s.clusterHealthChecker.HealthyCluster() {
			err = s.startNodeAsJoiner()
			newNodeState = Clustered
		} else {
			newNodeState = NeedsBootstrap
		}
	case Clustered:
		err = s.joinCluster()
		newNodeState = Clustered
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", state)
	}

	if err != nil {
		return "", err
	}

	if s.mysqlCmd != nil {
		s.waitForDatabaseToAcceptConnections()
		err = s.shutdownMysql()
		if err != nil {
			return "", err
		}
	}

	s.finalState = newNodeState

	return newNodeState, nil
}

func (s *prestarter) GetMysqlCmd() (*exec.Cmd, error) {
	if s.mysqlCmd != nil || (s.mysqlCmd == nil && s.finalState != Clustered) {
		return s.mysqlCmd, nil
	}
	return nil, errors.New("Mysql has not been started")
}

func (s *prestarter) startNodeAsJoiner() error {
	s.logger.Info("Joining an existing cluster")
	cmd, err := s.mariaDBHelper.StartMysqlInJoin()
	if err != nil {
		return err
	}

	s.mysqlCmd = cmd

	return nil
}

func (s *prestarter) joinCluster() (err error) {
	s.logger.Info("Joining a multi-node cluster")
	cmd, err := s.mariaDBHelper.StartMysqlInJoin()

	if err != nil {
		return err
	}

	s.mysqlCmd = cmd

	return nil
}

func (s *prestarter) waitForDatabaseToAcceptConnections() {
	s.logger.Info("Attempting to reach database.")
	numTries := 0
	for {
		if s.mariaDBHelper.IsDatabaseReachable() {
			s.logger.Info(fmt.Sprintf("Database became reachable after %d seconds", numTries*StartupPollingFrequencyInSeconds))
			return
		}
		s.logger.Info("Database not reachable, retrying...")
		s.osHelper.Sleep(StartupPollingFrequencyInSeconds * time.Second)
		numTries++
	}
}

func (s *prestarter) shutdownMysql() error {
	s.logger.Info("Shutting down MariaDB after prestart")
	return s.mariaDBHelper.StopMysql()
}
