package mysql_status

import (
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_status"
	"github.com/pivotal-golang/lager"
	"io/ioutil"
	"net/http"
	"net/url"
)

type MySQLStatus struct {
	config      config.MonitConfig
	logger      lager.Logger
	processName string
}

func New(monitConfig config.MonitConfig, logger lager.Logger) *MySQLStatus {
	return &MySQLStatus{
		config:      monitConfig,
		logger:      logger,
		processName: "mariadb_ctrl",
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

	statusURL, err := url.Parse(fmt.Sprintf("http://%s:%d/_status", config.Host, config.Port))

	if err != nil {
		mysqlstatus.logger.Error("Failed to parse URL", err)
		mysqlstatus.logger.Info("URL info", lager.Data{
			"URL": statusURL,
		})
		return "", err
	}

	urlValues := url.Values{}
	urlValues.Set("format", "xml")
	statusURL.RawQuery = urlValues.Encode()

	mysqlstatus.logger.Info("URL info", lager.Data{
		"url": statusURL.String(),
	})

	req, err := http.NewRequest("GET", statusURL.String(), nil)
	if err != nil {
		mysqlstatus.logger.Error("Failed to create http request", err)
		mysqlstatus.logger.Info("request info", lager.Data{
			"request": req.URL,
		})
		return "", err
	}

	req.SetBasicAuth(config.User, config.Password)

	resp, err := client.Do(req)
	if err != nil {
		mysqlstatus.logger.Error("Error sending http request", err)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		mysqlstatus.logger.Info("request and response info", lager.Data{
			"request":  req.URL,
			"response": string(responseBytes),
		})
		return "", err
	}

	if resp.StatusCode != 200 {
		non200Error := fmt.Errorf("Received %d response from monit", resp.StatusCode)
		mysqlstatus.logger.Error("Failed with non-200 response", non200Error)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		mysqlstatus.logger.Info("", lager.Data{
			"status_code":   resp.StatusCode,
			"response_body": string(responseBytes),
		})
		return "", non200Error
	}

	mysqlstatus.logger.Info("Made successful request to monit API")

	defer resp.Body.Close()
	if err != nil {
		mysqlstatus.logger.Error("Failed to read response body", err)
		return "", err
	}

	statusObject, err = statusObject.NewMonitStatus(resp.Body, mysqlstatus.logger)
	if err != nil {
		xmlParsingError := fmt.Errorf("Failed to parse XML")
		mysqlstatus.logger.Error(xmlParsingError.Error(), xmlParsingError)
		mysqlstatus.logger.Info("Response body Info", lager.Data{
			"status_code": resp.StatusCode,
		})
		return "", xmlParsingError
	}

	return statusObject.GetStatus(mysqlstatus.processName)
}
