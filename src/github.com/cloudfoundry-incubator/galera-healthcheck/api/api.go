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

type ApiParameters struct {
	RootConfig            *config.Config
	MonitClient           monit_client.MonitClient
	SequenceNumberChecker sequence_number.SequenceNumberChecker
	Healthchecker         healthcheck.HealthChecker
}

func NewRouter(apiParams ApiParameters) http.Handler {

	routes := rata.Routes{
		{Name: "mysql_status", Method: "GET", Path: "/mysql_status"},
		{Name: "stop_mysql", Method: "POST", Path: "/stop_mysql"},
		{Name: "start_mysql_bootstrap", Method: "POST", Path: "/start_mysql_bootstrap"},
		{Name: "start_mysql_join", Method: "POST", Path: "/start_mysql_join"},
		{Name: "sequence_number", Method: "GET", Path: "/sequence_number"},
		{Name: "galera_status", Method: "GET", Path: "/galera_status"},
		{Name: "root", Method: "GET", Path: "/"},
	}

	handlers := rata.Handlers{
		"mysql_status": getSecureHandler(func() (string, error) {
			return apiParams.MonitClient.GetStatus()
		}, apiParams),
		"stop_mysql": getSecureHandler(func() (string, error) {
			_, err := apiParams.MonitClient.StopService()
			return "", err
		}, apiParams),
		"start_mysql_bootstrap": getSecureHandler(func() (string, error) {
			_, err := apiParams.MonitClient.StartService("bootstrap")
			return "", err
		}, apiParams),
		"start_mysql_join": getSecureHandler(func() (string, error) {
			_, err := apiParams.MonitClient.StartService("join")
			return "", err
		}, apiParams),
		"sequence_number": getSecureHandler(func() (string, error) {
			return apiParams.SequenceNumberChecker.Check()
		}, apiParams),
		"galera_status": getInsecureHandler(func() (string, error) {
			return apiParams.Healthchecker.Check()
		}, apiParams),
		"root": getInsecureHandler(func() (string, error) {
			return apiParams.Healthchecker.Check()
		}, apiParams),
	}

	router, err := rata.NewRouter(routes, handlers)
	if err != nil {
		apiParams.RootConfig.Logger.Error("Error initializing router", err)
	}

	return router
}

func getSecureHandler(getResponse func() (string, error), apiParams ApiParameters) http.Handler {
	basicAuth := middleware.NewBasicAuth(
		apiParams.RootConfig.BootstrapEndpoint.Username,
		apiParams.RootConfig.BootstrapEndpoint.Password,
	)

	handler := getInsecureHandler(getResponse, apiParams)
	return basicAuth.Wrap(handler)
}

func getInsecureHandler(getResponse func() (string, error), apiParams ApiParameters) http.Handler {
	logger := apiParams.RootConfig.Logger
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := getResponse()
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
