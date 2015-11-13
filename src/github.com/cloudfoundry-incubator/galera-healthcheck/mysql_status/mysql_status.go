package mysql_status

import (
	"errors"
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_status"
	"github.com/pivotal-golang/lager"
	"io/ioutil"
	"net/http"
	"net/url"
)

type MySQLStatus struct {
	config config.MonitConfig
	logger lager.Logger
}

func New(monitConfig config.MonitConfig, logger lager.Logger) *MySQLStatus {
	return &MySQLStatus{
		config: monitConfig,
		logger: logger,
	}
}
func (mysqlstatus *MySQLStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	resp, err := mysqlstatus.MySQLStatusHandler()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to determine mariadb_ctrl process status: %s", err.Error())
		mysqlstatus.logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	mysqlstatus.logger.Debug(fmt.Sprintf("Response body: %s", resp))
	w.Write([]byte(resp))

}

func (mysqlstatus *MySQLStatus) MySQLStatusHandler() (string, error) {
	config := mysqlstatus.config
	var statusObject monit_status.MonitStatus
	client := &http.Client{}
	statusURL, err := url.Parse(fmt.Sprintf("http://%s:%d/", config.Host, config.Port))
	if err != nil {
		return "", err
	}

	urlValues := url.Values{}
	urlValues.Set("format", "xml")
	statusURL.RawQuery = urlValues.Encode()

	req, err := http.NewRequest("GET", statusURL.String(), nil)
	if err != nil {
		err = fmt.Errorf("Failed to create an http request %s", err.Error())
		return "", err
	}

	req.SetBasicAuth(config.User, config.Password)

	resp, err := client.Do(req)
	if err != nil {
		err = fmt.Errorf("Error sending http request %s", err.Error())
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", errors.New(fmt.Sprintf("Received non-200 response %s", resp.Body))
	}

	defer resp.Body.Close()
	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	statusObject, err = statusObject.NewMonitStatus(responseBytes)
	if err != nil {
		return "", err
	}

	return statusObject.GetStatus("mariadb_ctrl")
}
