package api

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api/middleware"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/tedsuo/rata"
)

//go:generate counterfeiter . ReqHealthChecker
type ReqHealthChecker interface {
	CheckReq(*http.Request) (string, error)
}

//go:generate counterfeiter . HealthChecker
type HealthChecker interface {
	Check() (string, error)
}

//go:generate counterfeiter . MonitClient
type MonitClient interface {
	StartServiceBootstrap(req *http.Request) (string, error)
	StartServiceJoin(req *http.Request) (string, error)
	StartServiceSingleNode(req *http.Request) (string, error)
	StopService(req *http.Request) (string, error)
	GetStatus(req *http.Request) (string, error)
	GetLogger(req *http.Request) lager.Logger
}

//go:generate counterfeiter . SequenceNumberChecker
type SequenceNumberChecker interface {
	Check(req *http.Request) (string, error)
}

type RunFunc func(req *http.Request) (string, error)

type router struct {
	logger                lager.Logger
	rootConfig            *config.Config
	monitClient           MonitClient
	sequenceNumberChecker SequenceNumberChecker
	reqHealthChecker      ReqHealthChecker
	healthchecker         HealthChecker
}

func NewRouter(
	logger lager.Logger,
	rootConfig *config.Config,
	monitClient MonitClient,
	sequenceNumberChecker SequenceNumberChecker,
	reqHealthChecker ReqHealthChecker,
	healthchecker HealthChecker,
) (http.Handler, error) {
	r := router{
		logger:                logger,
		rootConfig:            rootConfig,
		monitClient:           monitClient,
		sequenceNumberChecker: sequenceNumberChecker,
		reqHealthChecker:      reqHealthChecker,
		healthchecker:         healthchecker,
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
	}

	handlers := rata.Handlers{
		"v1_status": r.v1Status(),

		"mysql_status":            r.getSecureHandler(r.monitClient.GetStatus),
		"stop_mysql":              r.getSecureHandler(r.monitClient.StopService),
		"start_mysql_bootstrap":   r.getSecureHandler(r.monitClient.StartServiceBootstrap),
		"start_mysql_join":        r.getSecureHandler(r.monitClient.StartServiceJoin),
		"start_mysql_single_node": r.getSecureHandler(r.monitClient.StartServiceSingleNode),
		"sequence_number":         r.getSecureHandler(r.sequenceNumberChecker.Check),
		"galera_status":           r.getInsecureHandler(r.reqHealthChecker.CheckReq),
		"root":                    r.getInsecureHandler(r.reqHealthChecker.CheckReq),
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, err := r.healthchecker.Check()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			r.logger.Error("Failed to process request", err)
			w.Write([]byte(err.Error()))
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})

	return handler
}
