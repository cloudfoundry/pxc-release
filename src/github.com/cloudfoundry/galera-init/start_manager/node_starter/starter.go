package node_starter

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3"

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

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Starter
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

	if err = s.waitForDatabaseToAcceptConnections(mysqldChan); err != nil {
		return "", nil, err
	}

	// Once MySQL is reachable, we can scale up wsrep_applier_threads based on configuration,
	// even if the node is not yet "Synced"
	// This helps speed up nodes joining the cluster and general replication throughput and deferring this configuration
	// to post-startup avoids some race conditions between bootstrapping nodes and IST

	// TODO: In a production implementaiton, this defaulting logic should probably be pushed down to config parsing layer
	// pxc-release should default to wsrep_applier_threads # of cores if not overridden by the user with static configuration
	var wsrepApplierThreads = s.config.WsrepApplierThreads
	if wsrepApplierThreads <= 0 {
		// Use did not pick a value wsrep_applier_threads value, so default to number of available logical cores
		wsrepApplierThreads = runtime.NumCPU()
	}

	if err = s.dbHelper.SetVariable("wsrep_applier_threads", wsrepApplierThreads); err != nil {
		s.logger.Info("failed to set wsrep_applier_threads.  MySQL will default to one replication applier thread for cluster traffic.", lager.Data{"error": err.Error()})
	}
	s.logger.Info("Adjusted wsrep_applier_threads", lager.Data{"wsrep_applier_threads": wsrepApplierThreads})

	if err = s.waitForGaleraSynced(mysqldChan); err != nil {
		return "", nil, err
	}

	return newNodeState, mysqldChan, nil
}

func (s *starter) GetMysqlCmd() *exec.Cmd {
	return s.mysqlCmd
}

func (s *starter) bootstrapNode() (chan error, error) {
	s.logger.Info("Updating safe_to_bootstrap flag")
	read, err := os.ReadFile(s.config.GrastateFileLocation)
	if err == nil {
		subbed := strings.Replace(string(read), "safe_to_bootstrap: 0", "safe_to_bootstrap: 1", -1)
		err = os.WriteFile(s.config.GrastateFileLocation, []byte(subbed), 0777)
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

	var (
		numTries int
		start    = time.Now().UTC()
		ticker   = time.NewTicker(time.Second)
	)

	for {
		select {
		case <-mysqldChan:
			s.logger.Info("Database process exited, stop trying to connect to database")
			return errors.New("Mysqld exited with error; aborting. Review the mysqld error logs for more information.")
		case <-ticker.C:
			if s.dbHelper.IsDatabaseReachable() {
				s.logger.Info(fmt.Sprintf("Observed database available after %s", time.Since(start)))
				return nil
			}

			numTries++
			s.logger.Info("Still waiting: database is not online", lager.Data{"attempts": numTries, "wait_time": time.Since(start).String()})
		}
	}
}

func (s *starter) waitForGaleraSynced(mysqldChan chan error) error {
	s.logger.Info(fmt.Sprintf("Attempting to determine if database node has reached synced state"))

	var (
		numTries int
		start    = time.Now().UTC()
		ticker   = time.NewTicker(time.Second)
	)

	defer ticker.Stop()

	for {
		select {
		case <-mysqldChan:
			s.logger.Info("Database process exited while waiting for node to join cluster. Aborting.")
			return errors.New("Mysqld exited with error; aborting. Review the mysqld error logs for more information.")
		case <-ticker.C:
			if s.dbHelper.IsDatabaseSynced() {
				s.logger.Info(fmt.Sprintf("Observed database state as 'Synced' after %s", time.Since(start)))
				return nil
			}
			numTries++
			s.logger.Info("Still waiting: database is not synced.", lager.Data{"attempts": numTries, "wait_time": time.Since(start).String()})
		}
	}
}
