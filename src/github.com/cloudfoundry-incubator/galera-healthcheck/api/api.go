package api

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api/middleware"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
	"github.com/tedsuo/rata"
)

type RunFunc func() (string, error)

type ApiParameters struct {
	RootConfig            *config.Config
	MonitClient           monit_client.MonitClient
	SequenceNumberChecker sequence_number.SequenceNumberChecker
	Healthchecker         healthcheck.HealthChecker
}

type router struct {
	apiParams ApiParameters
}

func NewRouter(apiParams ApiParameters) (http.Handler, error) {

	r := router{
		apiParams: apiParams,
	}

	routes := rata.Routes{
		{Name: "mysql_status", Method: "GET", Path: "/mysql_status"},
		{Name: "stop_mysql", Method: "POST", Path: "/stop_mysql"},
		{Name: "start_mysql_bootstrap", Method: "POST", Path: "/start_mysql_bootstrap"},
		{Name: "start_mysql_join", Method: "POST", Path: "/start_mysql_join"},
		{Name: "start_mysql_single_node", Method: "POST", Path: "/start_mysql_single_node"},
		{Name: "sequence_number", Method: "GET", Path: "/sequence_number"},
		{Name: "galera_status", Method: "GET", Path: "/galera_status"},
		{Name: "root", Method: "GET", Path: "/"},
	}

	client := r.apiParams.MonitClient
	seqnoChecker := r.apiParams.SequenceNumberChecker
	healthchecker := r.apiParams.Healthchecker
	handlers := rata.Handlers{
		"mysql_status":            r.getSecureHandler(client.GetStatus),
		"stop_mysql":              r.getSecureHandler(client.StopService),
		"start_mysql_bootstrap":   r.getSecureHandler(client.StartServiceBootstrap),
		"start_mysql_join":        r.getSecureHandler(client.StartServiceJoin),
		"start_mysql_single_node": r.getSecureHandler(client.StartServiceSingleNode),
		"sequence_number":         r.getSecureHandler(seqnoChecker.Check),
		"galera_status":           r.getInsecureHandler(healthchecker.Check),
		"root":                    r.getInsecureHandler(healthchecker.Check),
	}

	handler, err := rata.NewRouter(routes, handlers)
	if err != nil {
		apiParams.RootConfig.Logger.Error("Error initializing router", err)
		return nil, err
	}

	return handler, nil
}

func (r router) getSecureHandler(run RunFunc) http.Handler {
	basicAuth := middleware.NewBasicAuth(
		r.apiParams.RootConfig.SidecarEndpoint.Username,
		r.apiParams.RootConfig.SidecarEndpoint.Password,
	)

	handler := r.getInsecureHandler(run)
	return basicAuth.Wrap(handler)
}

func (r router) getInsecureHandler(run RunFunc) http.Handler {
	logger := r.apiParams.RootConfig.Logger
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := run()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to process request", err)
			w.Write([]byte(err.Error()))
			return
		}

		logger.Debug(fmt.Sprintf("Response body: %s", body))
		w.Write([]byte(body))
	})
}
