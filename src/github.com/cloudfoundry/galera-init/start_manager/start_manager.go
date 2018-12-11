package start_manager

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/os_helper"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
	"github.com/cloudfoundry/galera-init/upgrader"
)

//go:generate counterfeiter . StartManager
type StartManager interface {
	Execute(ctx context.Context) error
	Shutdown()
}

//go:generate counterfeiter . ServiceStatus
type ServiceStatus interface {
	Start() error
}

type startManager struct {
	osHelper               os_helper.OsHelper
	config                 config.StartManager
	dbHelper               db_helper.DBHelper
	upgrader               upgrader.Upgrader
	startCaller            node_starter.Starter
	logger                 lager.Logger
	healthChecker          cluster_health_checker.ClusterHealthChecker
	mysqlCmd               *exec.Cmd
	mysqldPid              int
	galeraInitStatusServer ServiceStatus
}

func New(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	dbHelper db_helper.DBHelper,
	upgrader upgrader.Upgrader,
	startCaller node_starter.Starter,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
	galeraInitStatusServer ServiceStatus,
) StartManager {
	return &startManager{
		osHelper:               osHelper,
		config:                 config,
		logger:                 logger,
		dbHelper:               dbHelper,
		upgrader:               upgrader,
		startCaller:            startCaller,
		healthChecker:          healthChecker,
		galeraInitStatusServer: galeraInitStatusServer,
	}
}

func (m *startManager) Execute(ctx context.Context) error {
	var newNodeState string
	var err error

	if m.dbHelper.IsProcessRunning() {
		m.logger.Info("mysqld-already-running")
		m.logger.Info("shutdown-old-mysql")
		m.Shutdown()
	}

	needsUpgrade, err := m.upgrader.NeedsUpgrade()
	if err != nil {
		m.logger.Error("upgrade-check-failed", err)
		return err
	}
	if needsUpgrade {
		err = m.upgrader.Upgrade()
		if err != nil {
			m.logger.Error("mysql-upgrade-failed", err)
			return err
		}
	}

	m.logger.Info("determining-bootstrap-procedure", lager.Data{
		"ClusterIps":    m.config.ClusterIps,
		"BootstrapNode": m.config.BootstrapNode,
	})

	currentState, err := m.getCurrentNodeState()
	if err != nil {
		return err
	}

	var mysqldChan <-chan error

	newNodeState, mysqldChan, err = m.startCaller.StartNodeFromState(currentState)
	if err != nil {
		return err
	}

	err = m.writeStringToFile(newNodeState)
	if err != nil {
		return err
	}

	m.logger.Info("bootstrap-complete")
	m.logger.Info("waiting-for-mysqld")

	m.logger.Info("status-server-starting")
	m.galeraInitStatusServer.Start()
	m.logger.Info("status-server-started")

	select {
	case err := <-mysqldChan:
		m.logger.Info("mysqld-exited", lager.Data{
			"error": err,
		})
		return err
	case <-ctx.Done():
		m.logger.Info("shutdown-detected")

		err := m.osHelper.KillCommand(m.startCaller.GetMysqlCmd(), syscall.SIGTERM)
		if err != nil {
			m.logger.Error("sigterm-mysqld-failed", err)
			return err
		}
		m.logger.Info("sigterm-mysqld-ok")
		m.logger.Info("mysqld-shutdown-started")

		err = <-mysqldChan

		m.logger.Info("mysqld-shutdown-complete", lager.Data{
			"error": err,
		})

		return err
	}
}

func (m *startManager) getCurrentNodeState() (string, error) {

	// Single-node deploy always requires bootstrapping of new cluster
	if len(m.config.ClusterIps) == 1 {
		return node_starter.SingleNode, nil
	}

	if m.firstTimeDeploy() {
		if m.config.BootstrapNode {
			return node_starter.NeedsBootstrap, nil
		}

		return node_starter.Clustered, nil
	}

	// If we are not a first time deploy we must already have a state file
	state, err := m.readStateFromFile()
	if err != nil {
		m.logger.Info("state file could not be read", lager.Data{"err": err.Error()})
		return "", err
	}

	if state == node_starter.SingleNode && len(m.config.ClusterIps) > 1 {
		// Upgrading from a single-node cluster means we have to re-bootstrap
		return node_starter.NeedsBootstrap, nil
	}

	return state, nil
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

func (m *startManager) Shutdown() {
	m.logger.Info("Shutting down mysqld")
	m.dbHelper.StopMysqld()
}

func (m *startManager) writeStringToFile(contents string) error {
	m.logger.Info(fmt.Sprintf("updating file with contents: '%s'", contents))
	return m.osHelper.WriteStringToFile(m.config.StateFileLocation, contents)
}
