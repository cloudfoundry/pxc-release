package start_manager

import (
	"sync/atomic"
	"syscall"

	"code.cloudfoundry.org/lager/v3"
)

// Monit-compatible service status strings (for drop-in consumers).
const (
	MonitStatusRunning      = "running"
	MonitStatusStopped      = "stopped"
	MonitStatusInitializing = "initializing"
	MonitStatusFailing      = "failing"
	MonitStatusPending      = "pending"
)

// HTTP lifecycle and monit-compatibility (Phase 2)
//
// state.txt (Manager.StateFileLocation) is the source of truth for *desired* mode after the
// first deploy. The galera-agent (or operator) writes one of: NEEDS_BOOTSTRAP, CLUSTERED,
// SINGLE_NODE. On first boot there may be no file; getCurrentNodeState() then derives the
// intended mode from cluster size and BootstrapNode.
//
// POST /start (reconcileHTTPStart) reads that intent via getDesiredStateForReconcile, compares
// to lastAppliedMode, and is a no-op when the file matches what we already applied, mysqld is
// up, the DB is reachable, and readiness is not in the failed phase. Otherwise it stops
// mysqld (if needed) and runs StartNodeFromState, then writes the file with the return value
// from the starter (e.g. NEEDS_BOOTSTRAP run may persist CLUSTERED).
//
// POST /stop (reconcileHTTPStop) stops only the mysqld child; galera-init and the HTTP
// listener keep running. Readiness reports phase "stopped" until a later POST /start
// reconciles again.
//
// MonitStatusString and GET /v1/moniteq expose a monit-style string for drop-in healthcheck
// consumers. Internal readiness phase maps to that string as follows:
//
//   (empty)                  -> pending
//   failed                   -> failing
//   bootstrapping            -> initializing
//   running + DB reachable -> running
//   running, DB not yet OK   -> initializing
//   waiting_for_database     -> initializing
//   stopped                  -> stopped
//   (any other)              -> pending
//
// reconcileFirstBoot is the first reconcile on process start, before the main goroutine
// blocks on context cancellation; it does not go through the HTTP layer.

// reconcileFirstBoot runs the initial bootstrap in Execute. On error, Execute returns.
func (m *startManager) reconcileFirstBoot() error {
	if m.dbHelper.IsProcessRunning() {
		m.logger.Info("mysqld-already-running")
		m.logger.Info("shutdown-old-mysql")
		m.Shutdown()
	}
	m.logger.Info("determining-bootstrap-procedure", lager.Data{
		"ClusterIps":    m.config.ClusterIps,
		"BootstrapNode": m.config.BootstrapNode,
	})
	desired, err := m.getCurrentNodeState()
	if err != nil {
		m.setReadinessFailed(readinessPhaseFailed, "state", err.Error())
		return err
	}
	return m.applyMysqldStartWithDesiredState(desired)
}

// reconcileHTTPStart implements POST /start. See the HTTP lifecycle comment in this file.
func (m *startManager) reconcileHTTPStart() error {
	m.lifecycleMu.Lock()
	desired, err := m.getDesiredStateForReconcile()
	if err != nil {
		m.lifecycleMu.Unlock()
		return err
	}
	if m.noOpReconcileOKLocked(desired) {
		m.lifecycleMu.Unlock()
		return nil
	}
	m.lifecycleMu.Unlock()

	if m.dbHelper.IsProcessRunning() {
		if err := m.stopMysqldIntentional(); err != nil {
			return err
		}
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	desired, err = m.getDesiredStateForReconcile()
	if err != nil {
		m.setReadinessFailed(readinessPhaseFailed, "state", err.Error())
		return err
	}
	if m.noOpReconcileOKLocked(desired) {
		return nil
	}
	return m.applyMysqldStartWithDesiredStateLocked(desired)
}

// reconcileHTTPStop implements POST /stop. See the HTTP lifecycle comment in this file.
func (m *startManager) reconcileHTTPStop() error {
	m.lifecycleMu.Lock()
	if !m.dbHelper.IsProcessRunning() {
		m.setReadinessStoppedEmpty()
		m.lastAppliedMode = ""
		m.lifecycleMu.Unlock()
		return nil
	}
	m.lifecycleMu.Unlock()
	if err := m.stopMysqldIntentional(); err != nil {
		return err
	}
	return nil
}

// getDesiredStateForReconcile reads the same intent as the galera-agent: state file when
// it exists, otherwise the cluster config policy (e.g. first-time deploy with no file).
func (m *startManager) getDesiredStateForReconcile() (string, error) {
	if m.firstTimeDeploy() {
		return m.getCurrentNodeState()
	}
	return m.readStateFromFile()
}

// noOpReconcileOKLocked: desired mode already applied, process up, and DB is reachable.
// Caller must hold m.lifecycleMu.
func (m *startManager) noOpReconcileOKLocked(desired string) bool {
	m.readinessMu.RLock()
	phase := m.readiness.Phase
	m.readinessMu.RUnlock()
	if phase == readinessPhaseFailed {
		return false
	}
	if desired != m.lastAppliedMode {
		return false
	}
	if !m.dbHelper.IsProcessRunning() {
		return false
	}
	if !m.dbHelper.IsDatabaseReachable() {
		return false
	}
	return true
}

// applyMysqldStartWithDesiredState is used for first boot (takes the lifecycle lock).
func (m *startManager) applyMysqldStartWithDesiredState(desired string) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.applyMysqldStartWithDesiredStateLocked(desired)
}

// applyMysqldStartWithDesiredStateLocked: StartNode, write file, start watcher. Caller must hold m.lifecycleMu.
func (m *startManager) applyMysqldStartWithDesiredStateLocked(desired string) error {
	m.setReadinessReset(readinessPhaseBootstrapping, desired)

	newNodeState, mysqldCh, err := m.startCaller.StartNodeFromState(desired)
	if err != nil {
		m.setReadinessFailed(readinessPhaseFailed, "start", err.Error())
		return err
	}
	if err = m.writeStringToFile(newNodeState); err != nil {
		m.setReadinessFailed(readinessPhaseFailed, "statefile", err.Error())
		return err
	}
	m.lastAppliedMode = newNodeState
	m.currentMysqldCh = mysqldCh

	gen := atomic.AddUint64(&m.mysqldActiveGen, 1)
	m.spawnMysqldWatcher(mysqldCh, gen)
	m.setReadinessReset(readinessPhaseRunning, m.lastAppliedMode)
	return nil
}

// stopMysqldIntentional stops mysqld so we can re-reconcile. Only the watcher reads mysqldCh.
func (m *startManager) stopMysqldIntentional() error {
	m.lifecycleMu.Lock()
	if !m.dbHelper.IsProcessRunning() {
		m.lifecycleMu.Unlock()
		return nil
	}
	m.intentionalMysqldStop = true
	cmd := m.startCaller.GetMysqlCmd()
	if cmd == nil {
		m.intentionalMysqldStop = false
		m.lifecycleMu.Unlock()
		return nil
	}
	if err := m.osHelper.KillCommand(cmd, syscall.SIGTERM); err != nil {
		m.intentionalMysqldStop = false
		m.lifecycleMu.Unlock()
		return err
	}
	m.lifecycleMu.Unlock()
	m.mysqldWatchWg.Wait()
	return nil
}

// spawnMysqldWatcher: single goroutine reads mysqldCh per process.
func (m *startManager) spawnMysqldWatcher(mysqldCh <-chan error, gen uint64) {
	m.mysqldWatchWg.Add(1)
	go m.watchMysqld(mysqldCh, gen)
}

func (m *startManager) watchMysqld(mysqldCh <-chan error, myGen uint64) {
	defer m.mysqldWatchWg.Done()
	if mysqldCh == nil {
		return
	}
	exitErr, _ := <-mysqldCh
	if myGen != atomic.LoadUint64(&m.mysqldActiveGen) {
		return
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if myGen != atomic.LoadUint64(&m.mysqldActiveGen) {
		return
	}
	if atomic.LoadInt32(&m.shutdownInProgress) != 0 {
		m.currentMysqldCh = nil
		return
	}
	if m.intentionalMysqldStop {
		m.setReadinessStoppedEmpty()
		m.currentMysqldCh = nil
		m.intentionalMysqldStop = false
		m.lastAppliedMode = ""
		return
	}
	if exitErr != nil {
		m.setReadinessFailed(readinessPhaseFailed, "mysqld", exitErr.Error())
	} else {
		m.setReadinessFailed(readinessPhaseFailed, "mysqld", "mysqld process exited")
	}
	m.currentMysqldCh = nil
}

func (m *startManager) setReadinessStoppedEmpty() {
	m.setReadinessReset(readinessPhaseStopped, "")
}

// shutdownMysqldOnContextDone runs when galera-init receives SIGTERM. Stops child mysqld, waits for the watcher, then returns.
func (m *startManager) shutdownMysqldOnContextDone() error {
	atomic.StoreInt32(&m.shutdownInProgress, 1)
	m.logger.Info("shutdown-mysqld-on-context")
	m.lifecycleMu.Lock()
	if m.currentMysqldCh == nil {
		m.lifecycleMu.Unlock()
		return nil
	}
	cmd := m.startCaller.GetMysqlCmd()
	if cmd == nil {
		m.lifecycleMu.Unlock()
		return nil
	}
	if err := m.osHelper.KillCommand(cmd, syscall.SIGTERM); err != nil {
		m.lifecycleMu.Unlock()
		return err
	}
	m.lifecycleMu.Unlock()
	m.mysqldWatchWg.Wait()
	return nil
}

// MonitStatusString is the monit-compatible value for a single supervised service. See the
// HTTP lifecycle and monit-compatibility comment block in this file for the readiness-phase
// to monit-string table.
func (m *startManager) MonitStatusString(_ string) (string, error) {
	m.readinessMu.RLock()
	phase := m.readiness.Phase
	m.readinessMu.RUnlock()
	if phase == "" {
		return MonitStatusPending, nil
	}
	switch phase {
	case readinessPhaseFailed:
		return MonitStatusFailing, nil
	case readinessPhaseBootstrapping:
		return MonitStatusInitializing, nil
	case readinessPhaseRunning:
		if m.dbHelper.IsDatabaseReachable() {
			return MonitStatusRunning, nil
		}
		return MonitStatusInitializing, nil
	case readinessPhaseWaitingForDatabase:
		return MonitStatusInitializing, nil
	case readinessPhaseStopped:
		return MonitStatusStopped, nil
	default:
		return MonitStatusPending, nil
	}
}
