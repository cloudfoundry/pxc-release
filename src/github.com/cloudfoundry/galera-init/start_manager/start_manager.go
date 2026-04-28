package start_manager

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"code.cloudfoundry.org/lager/v3"

	"github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/galera_init_status_server"
	"github.com/cloudfoundry/galera-init/os_helper"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . StartManager
type StartManager interface {
	Execute(ctx context.Context) error
	Shutdown()
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . ServiceStatus
type ServiceStatus interface {
	Start() error
}

type startManager struct {
	mysqldActiveGen        uint64 // must be 64-bit aligned for atomic; generation of the current mysqld child
	lifecycleMu            sync.Mutex
	mysqldWatchWg          sync.WaitGroup
	osHelper               os_helper.OsHelper
	config                 config.StartManager
	dbHelper               db_helper.DBHelper
	startCaller            node_starter.Starter
	logger                 lager.Logger
	healthChecker          cluster_health_checker.ClusterHealthChecker
	galeraInitStatusServer ServiceStatus
	readinessMu            sync.RWMutex
	readiness              readinessData
	intentionalMysqldStop  bool
	shutdownInProgress     int32
	lastAppliedMode        string
	currentMysqldCh        <-chan error
}

func New(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	dbHelper db_helper.DBHelper,
	startCaller node_starter.Starter,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
	listener net.Listener,
) StartManager {
	m := newStartManagerCore(
		osHelper,
		config,
		dbHelper,
		startCaller,
		logger,
		healthChecker,
	)
	m.galeraInitStatusServer = galera_init_status_server.NewGaleraInitStatusServer(
		listener,
		m.httpHandler(),
		logger,
	)
	return m
}

// NewTest returns a [StartManager] for unit tests, using a fake or stub [ServiceStatus] instead of a real HTTP server.
func NewTest(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	dbHelper db_helper.DBHelper,
	startCaller node_starter.Starter,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
	galeraInitStatusServer ServiceStatus,
) StartManager {
	m := newStartManagerCore(
		osHelper,
		config,
		dbHelper,
		startCaller,
		logger,
		healthChecker,
	)
	m.galeraInitStatusServer = galeraInitStatusServer
	return m
}

func newStartManagerCore(
	osHelper os_helper.OsHelper,
	config config.StartManager,
	dbHelper db_helper.DBHelper,
	startCaller node_starter.Starter,
	logger lager.Logger,
	healthChecker cluster_health_checker.ClusterHealthChecker,
) *startManager {
	return &startManager{
		osHelper:      osHelper,
		config:        config,
		logger:        logger,
		dbHelper:      dbHelper,
		startCaller:   startCaller,
		healthChecker: healthChecker,
	}
}

func (m *startManager) Execute(ctx context.Context) error {
	m.setReadinessReset(readinessPhaseBootstrapping, "")

	m.logger.Info("status-server-starting")
	if err := m.galeraInitStatusServer.Start(); err != nil {
		return err
	}
	m.logger.Info("status-server-started")

	if err := m.reconcileFirstBoot(); err != nil {
		return err
	}

	<-ctx.Done()
	m.logger.Info("shutdown-detected")
	if err := m.shutdownMysqldOnContextDone(); err != nil {
		return err
	}
	m.logger.Info("mysqld-shutdown-complete")
	return nil
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
