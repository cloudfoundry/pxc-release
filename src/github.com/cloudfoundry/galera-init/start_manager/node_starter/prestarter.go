package node_starter

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
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
	var mysqldChan chan error

	switch state {
	case SingleNode:
		newNodeState = SingleNode
	case NeedsBootstrap:
		if s.clusterHealthChecker.HealthyCluster() {
			mysqldChan, err = s.startNodeAsJoiner()
			newNodeState = Clustered
		} else {
			newNodeState = NeedsBootstrap
		}
	case Clustered:
		mysqldChan, err = s.joinCluster()
		newNodeState = Clustered
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", state)
	}

	if err != nil {
		return "", err
	}

	if s.mysqlCmd != nil {
		if mysqldChan == nil {
			return "", errors.New("Starting mysql failed, no channel created - exiting")
		}
		err = s.waitForDatabaseToAcceptConnections(mysqldChan)

	}

	s.finalState = newNodeState

	return newNodeState, err
}

func (s *prestarter) GetMysqlCmd() (*exec.Cmd, error) {
	if s.mysqlCmd != nil || (s.mysqlCmd == nil && s.finalState != Clustered) {
		return s.mysqlCmd, nil
	}
	return nil, errors.New("mysqld has not been started")
}

func (s *prestarter) startNodeAsJoiner() (chan error, error) {
	s.logger.Info("Joining an existing cluster")
	cmd, err := s.mariaDBHelper.StartMysqldInJoin()
	if err != nil {
		return nil, err
	}

	s.mysqlCmd = cmd
	s.logger.Info("waiting for joining and existing cluster")
	errorChan := s.osHelper.WaitForCommand(cmd)
	s.logger.Info("mysqld exit")
	return errorChan, nil
}

func (s *prestarter) joinCluster() (chan error, error) {
	s.logger.Info("Joining a multi-node cluster")
	cmd, err := s.mariaDBHelper.StartMysqldInJoin()

	if err != nil {
		return nil, err
	}

	s.mysqlCmd = cmd

	s.logger.Info("waiting for multi-node cluster")
	errorChan := s.osHelper.WaitForCommand(cmd)
	s.logger.Info("mysqld exit")
	return errorChan, nil
}

func (s *prestarter) waitForDatabaseToAcceptConnections(mysqldChan chan error) error {
	s.logger.Info(fmt.Sprintf("Attempting to reach database."))
	numTries := 0
	for {
		numTries++

		select {
		case <-mysqldChan:
			s.logger.Info("Database process exited, stop trying to connecto to database")
			return errors.New("Mysqld exited with error; aborting. Review the mysqld error logs for more information.")
		default:
			if s.mariaDBHelper.IsDatabaseReachable() {
				s.logger.Info(fmt.Sprintf("Database became reachable after %d seconds", numTries*StartupPollingFrequencyInSeconds))
				return nil
			} else {
				s.logger.Info("Database not reachable, retrying...")
				s.osHelper.Sleep(StartupPollingFrequencyInSeconds * time.Second)
			}
		}
	}
}

func (s *prestarter) shutdownMysqld() error {
	s.logger.Info("Shutting down mysqld after prestart")
	return s.mariaDBHelper.StopMysqld()
}
