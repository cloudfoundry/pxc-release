package start_manager

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry/galera-init/galera_init_status_server"
)

const (
	readinessPhaseBootstrapping      = "bootstrapping"
	readinessPhaseFailed             = "failed"
	readinessPhaseRunning            = "running"
	readinessPhaseWaitingForDatabase = "waiting_for_database"
	readinessPhaseStopped            = "stopped"
)

type readinessData struct {
	Phase string
	Mode  string
	Error *galera_init_status_server.ErrorObject
}

// httpHandler registers POST /start, POST /stop, and GET / and GET /status.
func (m *startManager) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.HandlerFunc(m.handleReadiness))
	mux.Handle("GET /status", http.HandlerFunc(m.handleReadiness))
	mux.Handle("GET /v1/moniteq", http.HandlerFunc(m.handleMonitString))
	mux.Handle("POST /start", http.HandlerFunc(m.postStart))
	mux.Handle("POST /stop", http.HandlerFunc(m.postStop))
	return mux
}

func (m *startManager) snapshotReadiness() readinessData {
	m.readinessMu.RLock()
	defer m.readinessMu.RUnlock()
	out := m.readiness
	if out.Error != nil {
		copy := *out.Error
		out.Error = &copy
	}
	return out
}

func (m *startManager) setReadinessReset(phase, mode string) {
	m.readinessMu.Lock()
	defer m.readinessMu.Unlock()
	m.readiness = readinessData{Phase: phase, Mode: mode, Error: nil}
}

func (m *startManager) setReadinessFailed(phase, code, message string) {
	m.readinessMu.Lock()
	defer m.readinessMu.Unlock()
	m.readiness.Phase = phase
	if message != "" {
		m.readiness.Error = &galera_init_status_server.ErrorObject{Code: code, Message: message}
	} else {
		m.readiness.Error = nil
	}
}

// handleReadiness serves identical JSON for GET / and GET /status.
// Returns 200 when Galera/DB is reachable per the same logic as [db_helper.GaleraDBHelper.IsDatabaseReachable]
// and internal phase is past bootstrap; 503 while starting or waiting for the database.
func (m *startManager) handleReadiness(w http.ResponseWriter, r *http.Request) {
	snap := m.snapshotReadiness()
	resp := galera_init_status_server.ReadinessResponse{Mode: snap.Mode}

	if snap.Error != nil {
		resp.Ready = false
		resp.Phase = readinessPhaseFailed
		resp.Error = snap.Error
		m.writeReadinessStatus(w, http.StatusServiceUnavailable, &resp)
		return
	}

	if snap.Phase == readinessPhaseBootstrapping {
		resp.Ready = false
		resp.Phase = readinessPhaseBootstrapping
		m.writeReadinessStatus(w, http.StatusServiceUnavailable, &resp)
		return
	}

	if snap.Phase == readinessPhaseRunning {
		if m.dbHelper.IsDatabaseReachable() {
			resp.Ready = true
			resp.Phase = readinessPhaseRunning
			m.writeReadinessStatus(w, http.StatusOK, &resp)
			return
		}
		resp.Ready = false
		resp.Phase = readinessPhaseWaitingForDatabase
		m.writeReadinessStatus(w, http.StatusServiceUnavailable, &resp)
		return
	}

	if snap.Phase == readinessPhaseStopped {
		resp.Ready = false
		resp.Phase = readinessPhaseStopped
		m.writeReadinessStatus(w, http.StatusServiceUnavailable, &resp)
		return
	}

	// Unrecognized phase: treat as not ready
	resp.Ready = false
	if resp.Phase == "" {
		resp.Phase = snap.Phase
	}
	if resp.Phase == "" {
		resp.Phase = "unknown"
	}
	m.writeReadinessStatus(w, http.StatusServiceUnavailable, &resp)
}

func (m *startManager) writeReadinessStatus(
	w http.ResponseWriter,
	status int,
	body *galera_init_status_server.ReadinessResponse,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (m *startManager) handleMonitString(w http.ResponseWriter, r *http.Request) {
	s, _ := m.MonitStatusString("")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s))
}

func (m *startManager) postStart(w http.ResponseWriter, r *http.Request) {
	m.logger.Info("http-post-start-received")
	if err := m.reconcileHTTPStart(); err != nil {
		m.writeAckError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(&galera_init_status_server.AckResponse{OK: true})
}

func (m *startManager) postStop(w http.ResponseWriter, r *http.Request) {
	m.logger.Info("http-post-stop-received")
	if err := m.reconcileHTTPStop(); err != nil {
		m.writeAckError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(&galera_init_status_server.AckResponse{OK: true})
}

func (m *startManager) writeAckError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(&galera_init_status_server.AckResponse{
		OK:    false,
		Error: &galera_init_status_server.ErrorObject{Code: "reconcile", Message: err.Error()},
	})
}
