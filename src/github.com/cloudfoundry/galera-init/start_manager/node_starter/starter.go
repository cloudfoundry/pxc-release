package node_starter

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
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
	Halted                           = "HALTED"
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
	case Halted:
		mysqldChan, err = s.startMysqldWithRecovery()
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

func (s *starter) recoverSeqno() (string, error) {
	s.logger.Info("Recovering sequence number using mysqld --wsrep-recover")
	
	errorLogFile := path.Join(os.TempDir(), "galera-init-mysqld-log.err")
	os.RemoveAll(errorLogFile) // ensure log is empty

	cmd := exec.Command("mysqld",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
		"--wsrep-recover",
		fmt.Sprintf("--log-error=%s", errorLogFile))

	stdout, cmdErr := cmd.CombinedOutput()
	stderr, readingLogErr := ioutil.ReadFile(errorLogFile)
	if readingLogErr != nil {
		stderr = []byte("failed to read stderr")
	}

	if cmdErr != nil {
		s.logger.Error("Error running mysqld recovery", cmdErr, lager.Data{
			"stdout": string(stdout),
			"stderr": string(stderr),
		})
		return "", cmdErr
	} else {
		s.logger.Debug(string(stdout))
	}

	seqNoRegex := `WSREP. Recovered position:.*:(-?\d+)`
	re := regexp.MustCompile(seqNoRegex)
	sequenceNumberLogLine := re.FindStringSubmatch(string(stderr))

	if len(sequenceNumberLogLine) < 2 {
		// First match is the whole string, second match is the seq no
		err := errors.New(fmt.Sprintf("Couldn't find regex: %s Log Line: %s", seqNoRegex, sequenceNumberLogLine))
		s.logger.Error("Failed to parse seqno from logs", err)
		return "", err
	}

	sequenceNumber := sequenceNumberLogLine[1]
	s.logger.Info(fmt.Sprintf("Recovered sequence number: %s", sequenceNumber))
	return sequenceNumber, nil
}

func (s *starter) startMysqldWithRecovery() (chan error, error) {
	s.logger.Info("Starting mysqld with sequence number recovery (HALTED mode)")
	
	// Recover sequence number first
	seqno, err := s.recoverSeqno()
	if err != nil {
		s.logger.Error("Failed to recover sequence number", err)
		return nil, err
	}

	// Start MySQL in join mode with recovered sequence position
	var mysqldArgs []string
	if seqno != "" && seqno != "-1" {
		// Use the recovered sequence number to set wsrep start position
		// Format as UUID:seqno - we'll use a placeholder UUID since we only have seqno
		mysqldArgs = append(mysqldArgs, "--wsrep-start-position=00000000-0000-0000-0000-000000000000:"+seqno)
	}

	cmd, err := s.startMysqldAsChildProcess(mysqldArgs...)
	if err != nil {
		s.logger.Info(fmt.Sprintf("Error starting mysqld with recovery: %s", err.Error()))
		return nil, err
	}
	s.mysqlCmd = cmd
	s.logger.Info("Issuing a non-blocking Wait for mysqld in recovery mode")
	errorChan := s.osHelper.WaitForCommand(cmd)
	return errorChan, nil
}

func (s *starter) startMysqldAsChildProcess(mysqlArgs ...string) (*exec.Cmd, error) {
	args := append(
		[]string{
			"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
			"--defaults-group-suffix=_plugin",
		},
		mysqlArgs...,
	)
	return s.osHelper.StartCommand(s.config.LogFileLocation, "mysqld", args...)
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
