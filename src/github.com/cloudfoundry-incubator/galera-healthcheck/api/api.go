package api

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
	"github.com/pivotal-golang/lager"
)

type ApiParameters struct {
	RootConfig            *config.Config
	MonitClient           monit_client.MonitClient
	SequenceNumberChecker sequence_number.SequenceNumberChecker
	Healthchecker         healthcheck.HealthChecker
}

func NewHandler(apiParams ApiParameters) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/mysql_status", getHandlerFromFunc(func() (string, error) {
		return apiParams.MonitClient.GetStatus()
	}, apiParams.RootConfig.Logger))

	mux.Handle("/stop_mysql", getHandlerFromFunc(func() (string, error) {
		_, err := apiParams.MonitClient.StopService()
		return "", err
	}, apiParams.RootConfig.Logger))

	mux.Handle("/start_mysql_bootstrap", getHandlerFromFunc(func() (string, error) {
		_, err := apiParams.MonitClient.StartService("bootstrap")
		return "", err
	}, apiParams.RootConfig.Logger))

	mux.Handle("/start_mysql_join", getHandlerFromFunc(func() (string, error) {
		_, err := apiParams.MonitClient.StartService("join")
		return "", err
	}, apiParams.RootConfig.Logger))

	mux.Handle("/sequence_number", getHandlerFromFunc(func() (string, error) {
		return apiParams.SequenceNumberChecker.Check()
	}, apiParams.RootConfig.Logger))

	mux.Handle("/galera_status", getHandlerFromFunc(func() (string, error) {
		return apiParams.Healthchecker.Check()
	}, apiParams.RootConfig.Logger))

	mux.Handle("/", getHandlerFromFunc(func() (string, error) {
		return apiParams.Healthchecker.Check()
	}, apiParams.RootConfig.Logger))

	return mux
}

func getHandlerFromFunc(getResponse func() (string, error), logger lager.Logger) http.Handler {
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
