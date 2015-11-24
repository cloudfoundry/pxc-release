package bootstrap_api

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	"github.com/pivotal-golang/lager"
)

func NewHandler(rootConfig *config.Config, monitClient monit_client.MonitClient) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/mysql_status", getHandlerFromFunc(func() (string, error) {
		return monitClient.GetStatus()
	}, rootConfig.Logger))

	mux.Handle("/stop_mysql", getHandlerFromFunc(func() (string, error) {
		_, err := monitClient.StopService()
		return "", err
	}, rootConfig.Logger))

	mux.Handle("/start_mysql_bootstrap", getHandlerFromFunc(func() (string, error) {
		_, err := monitClient.StartService("bootstrap")
		return "", err
	}, rootConfig.Logger))

	mux.Handle("/start_mysql_join", getHandlerFromFunc(func() (string, error) {
		_, err := monitClient.StartService("join")
		return "", err
	}, rootConfig.Logger))

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
