package node_starter

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/os_helper"
)

const (
	Clustered                        = "CLUSTERED"
	NeedsBootstrap                   = "NEEDS_BOOTSTRAP"
	SingleNode                       = "SINGLE_NODE"
	StartupPollingFrequencyInSeconds = 5
)

//go:generate counterfeiter . Starter
type Starter interface {
	StartNodeFromState(string) (string, <-chan error, error)
	GetMysqlCmd() *exec.Cmd
}

type starter struct {
	dbHelper             db_helper.DBHelper
	osHelper             os_helper.OsHelper
	clusterHealthChecker cluster_health_checker.ClusterHealthChecker
	config               config.StartManager
	logger               lager.Logger
	mysqlCmd             *exec.Cmd
}

func NewStarter(
	dbHelper db_helper.DBHelper,
	osHelper os_helper.OsHelper,
	config config.StartManager,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
) Starter {
	return &starter{
		dbHelper:             dbHelper,
		osHelper:             osHelper,
		config:               config,
		logger:               logger,
		clusterHealthChecker: healthChecker,
	}
}

func (s *starter) StartNodeFromState(state string) (string, <-chan error, error) {
	var newNodeState string
	var err error
	var mysqldChan chan error

	switch state {
	case SingleNode:
		mysqldChan, err = s.bootstrapNode()
		newNodeState = SingleNode
	case NeedsBootstrap:
		if s.clusterHealthChecker.HealthyCluster() {
			mysqldChan, err = s.joinCluster()
		} else {
			mysqldChan, err = s.bootstrapNode()
		}
		newNodeState = Clustered
	case Clustered:
		mysqldChan, err = s.joinCluster()
		newNodeState = Clustered
	default:
		err = fmt.Errorf("Unsupported state file contents: %s", state)
	}
	if err != nil {
		return "", nil, err
	}
	if mysqldChan == nil {
		return "", nil, errors.New("Starting mysql failed, no channel created - exiting")
	}

	err = s.waitForDatabaseToAcceptConnections(mysqldChan)
	if err != nil {
		return "", nil, err
	}

	err = s.seedDatabases()
	if err != nil {
		return "", nil, err
	}

	err = s.runPostStartSQL()
	if err != nil {
		return "", nil, err
	}

	err = s.runTestDatabaseCleanup()
	if err != nil {
		return "", nil, err
	}

	return newNodeState, mysqldChan, nil
}

func (s *starter) GetMysqlCmd() *exec.Cmd {
	return s.mysqlCmd
}

func (s *starter) bootstrapNode() (chan error, error) {
	s.logger.Info("Updating safe_to_bootstrap flag")
	read, err := ioutil.ReadFile(s.config.GrastateFileLocation)
	if err == nil {
		subbed := strings.Replace(string(read), "safe_to_bootstrap: 0", "safe_to_bootstrap: 1", -1)
		err = ioutil.WriteFile(s.config.GrastateFileLocation, []byte(subbed), 0777)
		if err != nil {
			return nil, err
		}
	}

	s.logger.Info("Bootstrapping node")
	cmd, err := s.dbHelper.StartMysqldInBootstrap()
	if err != nil {
		return nil, err
	}
	s.mysqlCmd = cmd
	s.logger.Info("Issusing a non-blocking Wait for mysqld in bootstrapping mode")
	errorChan := s.osHelper.WaitForCommand(cmd)
	return errorChan, nil
}

func (s *starter) joinCluster() (chan error, error) {
	s.logger.Info("Joining a multi-node cluster")
	cmd, err := s.dbHelper.StartMysqldInJoin()

	if err != nil {
		return nil, err
	}

	s.mysqlCmd = cmd
	s.logger.Info("Issueing a non-blocking Wait for mysqld in join cluster mode")
	mysqldChan := s.osHelper.WaitForCommand(cmd)

	return mysqldChan, nil
}

func (s *starter) waitForDatabaseToAcceptConnections(mysqldChan chan error) error {
	s.logger.Info(fmt.Sprintf("Attempting to reach database."))
	numTries := 0

	for {
		numTries++

		select {
		case <-mysqldChan:
			s.logger.Info("Database process exited, stop trying to connect to database")
			return errors.New("Mysqld exited with error; aborting. Review the mysqld error logs for more information.")
		default:
			if s.dbHelper.IsDatabaseReachable() {
				s.logger.Info(fmt.Sprintf("Database became reachable after %d seconds", numTries*StartupPollingFrequencyInSeconds))
				return nil
			} else {
				s.logger.Info("Database not reachable, retrying...")
				s.osHelper.Sleep(StartupPollingFrequencyInSeconds * time.Second)
			}
		}
	}
}

func (s *starter) seedDatabases() error {
	err := s.dbHelper.Seed()
	if err != nil {
		s.logger.Info(fmt.Sprintf("There was a problem seeding the database: '%s'", err.Error()))
		return err
	}

	s.logger.Info("Seeding databases succeeded.")
	return nil
}

func (s *starter) runPostStartSQL() error {
	err := s.dbHelper.RunPostStartSQL()
	if err != nil {
		s.logger.Info(fmt.Sprintf("There was a problem running post start sql: '%s'", err.Error()))
		return err
	}

	s.logger.Info("Post start sql succeeded.")
	return nil
}

func (s *starter) runTestDatabaseCleanup() error {
	err := s.dbHelper.TestDatabaseCleanup()
	if err != nil {
		s.logger.Info("There was a problem cleaning up test databases", lager.Data{
			"errMessage": err.Error(),
		})
		return err
	}

	s.logger.Info("Test database cleanup succeeded.")
	return nil
}
