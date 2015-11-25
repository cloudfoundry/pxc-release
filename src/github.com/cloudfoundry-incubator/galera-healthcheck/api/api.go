package api

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api/middleware"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
)

type ApiParameters struct {
	RootConfig            *config.Config
	MonitClient           monit_client.MonitClient
	SequenceNumberChecker sequence_number.SequenceNumberChecker
	Healthchecker         healthcheck.HealthChecker
}

func NewHandler(apiParams ApiParameters) *http.ServeMux {

	mux := http.NewServeMux()

	mux.Handle("/mysql_status", getSecureHandler(func() (string, error) {
		return apiParams.MonitClient.GetStatus()
	}, apiParams))

	mux.Handle("/stop_mysql", getSecureHandler(func() (string, error) {
		_, err := apiParams.MonitClient.StopService()
		return "", err
	}, apiParams))

	mux.Handle("/start_mysql_bootstrap", getSecureHandler(func() (string, error) {
		_, err := apiParams.MonitClient.StartService("bootstrap")
		return "", err
	}, apiParams))

	mux.Handle("/start_mysql_join", getSecureHandler(func() (string, error) {
		_, err := apiParams.MonitClient.StartService("join")
		return "", err
	}, apiParams))

	mux.Handle("/sequence_number", getSecureHandler(func() (string, error) {
		return apiParams.SequenceNumberChecker.Check()
	}, apiParams))

	mux.Handle("/galera_status", getInsecureHandler(func() (string, error) {
		return apiParams.Healthchecker.Check()
	}, apiParams))

	mux.Handle("/", getInsecureHandler(func() (string, error) {
		return apiParams.Healthchecker.Check()
	}, apiParams))

	return mux
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
