package monit_cmd

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
)

type StopMysqlCmd struct {
	monitClient monit_client.MonitClient
}

type StartMysqlCmd struct {
	monitClient monit_client.MonitClient
	startMode   string
}

type GetStatusCmd struct {
	monitClient monit_client.MonitClient
}

func NewStartMysqlCmd(monitClient monit_client.MonitClient, startMode string) *StartMysqlCmd {
	return &StartMysqlCmd{
		monitClient: monitClient,
		startMode:   startMode,
	}
}

func NewStopMysqlCmd(monitClient monit_client.MonitClient) *StopMysqlCmd {
	return &StopMysqlCmd{
		monitClient: monitClient,
	}
}

func NewGetStatusCmd(monitClient monit_client.MonitClient) *GetStatusCmd {
	return &GetStatusCmd{
		monitClient: monitClient,
	}
}

func (cmd *GetStatusCmd) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := cmd.monitClient.GetLogger()

	resp, err := cmd.monitClient.GetStatus()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to stop: %s", err.Error())
		logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	logger.Debug(fmt.Sprintf("Response body: %s", resp))
	w.Write([]byte(resp))
}

func (cmd *StopMysqlCmd) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := cmd.monitClient.GetLogger()

	resp, err := cmd.monitClient.StopService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to stop: %s", err.Error())
		logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	logger.Debug(fmt.Sprintf("Response body: %t", resp))
	w.Write([]byte(fmt.Sprintf("MySQLService Stopped Successfully")))
}

func (cmd *StartMysqlCmd) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := cmd.monitClient.GetLogger()

	resp, err := cmd.monitClient.StartService(cmd.startMode)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to start: %s", err.Error())
		logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	logger.Debug(fmt.Sprintf("Response body: %t", resp))
	w.Write([]byte(fmt.Sprintf("MySQLService Started Successfully")))
}
