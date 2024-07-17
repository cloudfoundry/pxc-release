package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/lager/v3"

	"github.com/tedsuo/rata"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api/middleware"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . HealthChecker
type HealthChecker interface {
	Check() (string, error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . StateSnapshotter
type StateSnapshotter interface {
	State() (domain.DBState, error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . MonitClient
type MonitClient interface {
	StartServiceBootstrap(req *http.Request) (string, error)
	StartServiceJoin(req *http.Request) (string, error)
	StartServiceSingleNode(req *http.Request) (string, error)
	StopService(req *http.Request) (string, error)
	GetStatus(req *http.Request) (string, error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . SequenceNumberChecker
type SequenceNumberChecker interface {
	Check(req *http.Request) (string, error)
}

type RunFunc func(req *http.Request) (string, error)

type router struct {
	logger                lager.Logger
	rootConfig            *config.Config
	monitClient           MonitClient
	sequenceNumberChecker SequenceNumberChecker
	healthchecker         HealthChecker
	stateSnapshotter      StateSnapshotter
}

func NewRouter(
	logger lager.Logger,
	rootConfig *config.Config,
	monitClient MonitClient,
	sequenceNumberChecker SequenceNumberChecker,
	healthchecker HealthChecker,
	stateSnapshotter StateSnapshotter,
) (http.Handler, error) {
	r := router{
		logger:                logger,
		rootConfig:            rootConfig,
		monitClient:           monitClient,
		sequenceNumberChecker: sequenceNumberChecker,
		healthchecker:         healthchecker,
		stateSnapshotter:      stateSnapshotter,
	}

	routes := rata.Routes{
		{Name: "v1_status", Method: "GET", Path: "/api/v1/status"},

		{Name: "mysql_status", Method: "GET", Path: "/mysql_status"},
		{Name: "stop_mysql", Method: "POST", Path: "/stop_mysql"},
		{Name: "start_mysql_bootstrap", Method: "POST", Path: "/start_mysql_bootstrap"},
		{Name: "start_mysql_join", Method: "POST", Path: "/start_mysql_join"},
		{Name: "start_mysql_single_node", Method: "POST", Path: "/start_mysql_single_node"},
		{Name: "sequence_number", Method: "GET", Path: "/sequence_number"},
		{Name: "galera_status", Method: "GET", Path: "/galera_status"},
		{Name: "root", Method: "GET", Path: "/"},

		{Name: "health", Method: "GET", Path: "/health"},
	}

	handlers := rata.Handlers{
		"v1_status": r.v1Status(),

		"mysql_status":            r.getSecureHandler(r.monitClient.GetStatus),
		"stop_mysql":              r.getSecureHandler(r.monitClient.StopService),
		"start_mysql_bootstrap":   r.getSecureHandler(r.monitClient.StartServiceBootstrap),
		"start_mysql_join":        r.getSecureHandler(r.monitClient.StartServiceJoin),
		"start_mysql_single_node": r.getSecureHandler(r.monitClient.StartServiceSingleNode),
		"sequence_number":         r.getSecureHandler(r.sequenceNumberChecker.Check),
		"galera_status":           r.getInsecureHandler(func(req *http.Request) (string, error) { return r.healthchecker.Check() }),
		"root":                    r.getInsecureHandler(func(req *http.Request) (string, error) { return r.healthchecker.Check() }),
		"health":                  r.getInsecureHandler(func(req *http.Request) (string, error) { return "", nil }),
	}

	handler, err := rata.NewRouter(routes, handlers)
	if err != nil {
		logger.Error("Error initializing router", err)
		return nil, err
	}

	return handler, nil
}

func (r router) getSecureHandler(run RunFunc) http.Handler {
	basicAuth := middleware.NewBasicAuth(
		r.rootConfig.SidecarEndpoint.Username,
		r.rootConfig.SidecarEndpoint.Password,
	)

	handler := r.getInsecureHandler(run)
	return basicAuth.Wrap(handler)
}

func (r router) getInsecureHandler(run RunFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := run(req)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			r.logger.Error("Failed to process request", err)
			w.Write([]byte(err.Error()))
			return
		}

		r.logger.Debug(fmt.Sprintf("Response body: %s", body))
		w.Write([]byte(body))
	})
}

func (r router) v1Status() http.Handler {
	var priorHealth bool
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s, err := r.stateSnapshotter.State()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			r.logger.Error("Failed to process request", err)
			w.Write([]byte(err.Error()))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		currentHealth := s.IsHealthy(r.rootConfig.AvailableWhenReadOnly)
		resp := V1StatusResponse{
			WsrepLocalState:        uint(s.WsrepLocalState),
			WsrepLocalStateComment: string(s.WsrepLocalState.Comment()),
			WsrepLocalIndex:        s.WsrepLocalIndex,
			Healthy:                currentHealth,
		}

		if priorHealth != currentHealth {
			r.logger.Info(fmt.Sprintf("health transition response: %#v maintenanceEnabled: %t readOnly: %t", resp, s.MaintenanceEnabled, s.ReadOnly))
		}
		priorHealth = currentHealth
		json.NewEncoder(w).Encode(resp)
	})
}

type V1StatusResponse struct {
	WsrepLocalState        uint   `json:"wsrep_local_state"`
	WsrepLocalStateComment string `json:"wsrep_local_state_comment"`
	WsrepLocalIndex        uint   `json:"wsrep_local_index"`
	Healthy                bool   `json:"healthy"`
}
