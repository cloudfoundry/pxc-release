package monit_client

import (
	"bytes"
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/pivotal-golang/lager"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Monit_Client struct {
	monitConfig config.MonitConfig
	logger      lager.Logger
	serviceName string
}

func New(monitConfig config.MonitConfig, logger lager.Logger, serviceName string) *Monit_Client {
	return &Monit_Client{
		monitConfig: monitConfig,
		logger:      logger,
		serviceName: serviceName,
	}
}

func (monitClient *Monit_Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	resp, err := monitClient.StopService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to determine mariadb_ctrl process status: %s", err.Error())
		monitClient.logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	monitClient.logger.Debug(fmt.Sprintf("Response body: %t", resp))
	w.Write([]byte(fmt.Sprintf("MySQLService Stopped Successfully. Req URL: %s", r.URL.String())))
}

func (monitClient *Monit_Client) StopService() (bool, error) {
	config := monitClient.monitConfig
	client := &http.Client{}

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

	var jsonStr = []byte(`action=unmonitor`)
	req, err := http.NewRequest("POST", statusURL.String(), bytes.NewReader(jsonStr))

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
	stopSuccessful := "running - unmonitor pending"
	if !strings.Contains(responseStr, stopSuccessful) {
		monitStopFailure := fmt.Errorf("Monit failed to stop %s successfully", monitClient.serviceName)
		monitClient.logger.Error("Monit failure:", monitStopFailure)
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		monitClient.logger.Info("request info", lager.Data{
			"response_body": string(responseBytes),
		})

		return false, monitStopFailure
	}

	defer resp.Body.Close()

	return true, nil
}
