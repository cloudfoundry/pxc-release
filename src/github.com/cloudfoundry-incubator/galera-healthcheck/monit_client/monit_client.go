package monit_client

import (
	"bytes"
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysql_start_mode"
	"github.com/pivotal-golang/lager"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type MonitClient interface {
	StartService(startMode string) (bool, error)
	StopService() (bool, error)
	GetLogger() lager.Logger
}

type monitClient struct {
	monitConfig config.MonitConfig
	logger      lager.Logger
	serviceName string
}

func New(monitConfig config.MonitConfig, logger lager.Logger, serviceName string) *monitClient {
	return &monitClient{
		monitConfig: monitConfig,
		logger:      logger,
		serviceName: serviceName,
	}
}

func (monitClient *monitClient) GetLogger() lager.Logger {
	return monitClient.logger
}

func (monitClient *monitClient) StartService(startMode string) (bool, error) {

	mySqlStartMode := mysql_start_mode.NewMysqlStartMode(monitClient.monitConfig.MysqlStateFilePath, startMode)
	_, err := mySqlStartMode.Start()
	if err != nil {
		monitClient.logger.Error("Failed to write state file", err)
		monitClient.logger.Info("mySqlStartMode info", lager.Data{
			"startMode":          startMode,
			"MysqlStateFilePath": monitClient.monitConfig.MysqlStateFilePath,
		})
		return false, err
	}

	resp, err := monitClient.runServiceCmd("monitor", "not monitored - monitor pending")
	return resp, err
}

func (monitClient *monitClient) StopService() (bool, error) {
	resp, err := monitClient.runServiceCmd("unmonitor", "running - unmonitor pending")
	return resp, err
}

func (monitClient *monitClient) runServiceCmd(command string, expectedSuccessResponse string) (bool, error) {
	config := monitClient.monitConfig
	client := &http.Client{}
	var serviceAction = []byte(fmt.Sprintf(`action=%s`, command))

	statusURL, err := url.Parse(fmt.Sprintf("http://%s:%d/%s",
		config.Host,
		config.Port,
		monitClient.serviceName,
	))

	if err != nil {
		monitClient.logger.Error("Failed to parse URL", err)
		monitClient.logger.Info("URL info", lager.Data{
			"URL": statusURL,
		})
		return false, err
	}

	urlValues := url.Values{}
	statusURL.RawQuery = urlValues.Encode()
	req, err := http.NewRequest("POST", statusURL.String(), bytes.NewReader(serviceAction))

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		monitClient.logger.Error("Failed to create http request", err)
		monitClient.logger.Info("request info", lager.Data{
			"request": req.URL,
		})
		return false, err
	}

	req.SetBasicAuth(config.User, config.Password)

	resp, err := client.Do(req)

	if err != nil {
		monitClient.logger.Error("Error sending http request", err)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		monitClient.logger.Info("request and response info", lager.Data{
			"request":  req.URL,
			"response": string(responseBytes),
		})
		return false, err
	}

	if resp.StatusCode != 200 {
		non200Error := fmt.Errorf("Received %d response from monit", resp.StatusCode)
		monitClient.logger.Error("Failed with non-200 response", non200Error)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		monitClient.logger.Info("", lager.Data{
			"status_code":   resp.StatusCode,
			"response_body": string(responseBytes),
		})
		return false, non200Error
	}

	monitClient.logger.Info("Made successful request to monit API")
	responseBytes, _ := ioutil.ReadAll(resp.Body)
	responseStr := string(responseBytes)

	if !strings.Contains(responseStr, expectedSuccessResponse) {
		monitFailure := fmt.Errorf("Monit failed to %s %s successfully", command, monitClient.serviceName)
		monitClient.logger.Error("Monit failure:", monitFailure)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		monitClient.logger.Info("request info", lager.Data{
			"response_body": string(responseBytes),
		})

		return false, monitFailure
	}

	defer resp.Body.Close()

	return true, nil
}
