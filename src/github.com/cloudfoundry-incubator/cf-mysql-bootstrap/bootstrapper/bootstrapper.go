package bootstrapper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-golang/lager"
)

type Bootstrapper struct {
	rootConfig *config.Config
	clock      clock.Clock
}

func New(rootConfig *config.Config, clock clock.Clock) *Bootstrapper {
	return &Bootstrapper{
		rootConfig: rootConfig,
		clock:      clock,
	}
}

func (b *Bootstrapper) Run() error {
	logger := b.rootConfig.Logger

	for _, url := range b.rootConfig.HealthcheckURLs {
		stopMysqlUrl := fmt.Sprintf("%s/%s", url, b.rootConfig.ShutDownMysql)
		resp, err := http.Get(stopMysqlUrl)
		if err != nil {
			return fmt.Errorf("Failed to stop mysql: %s", err.Error())
		}

		if resp.StatusCode != http.StatusOK {
			responseBody := ""
			if resp.Body != nil {
				responseBytes, _ := ioutil.ReadAll(resp.Body)
				responseBody = string(responseBytes)
			}
			return fmt.Errorf("Non 200 response from stopping mysql at %s: %s", stopMysqlUrl, responseBody)
		}

		logger.Info(fmt.Sprintf("Successfully sent stop request to URL: %s", stopMysqlUrl))
	}

	pollingIntervalInSec := 3
	timeoutInSec := 30
	maxIterations := timeoutInSec / pollingIntervalInSec
	for _, url := range b.rootConfig.HealthcheckURLs {
		statusUrl := fmt.Sprintf("%s/%s", url, b.rootConfig.MysqlStatus)
		stoppedSuccessfully := false

		for i := 0; i < maxIterations; i++ {
			resp, err := http.Get(statusUrl)
			if err != nil {
				return fmt.Errorf("Failed to get mysql status: %s", err.Error())
			}

			responseBody := ""
			if resp.Body != nil {
				responseBytes, _ := ioutil.ReadAll(resp.Body)
				responseBody = string(responseBytes)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Received non-200 response when getting mysql status: %s", responseBody)
			}

			if responseBody == "stopped" {
				stoppedSuccessfully = true
				break
			}

			<-b.clock.After(time.Duration(pollingIntervalInSec) * time.Second)
		}

		if stoppedSuccessfully == false {
			return fmt.Errorf("Timed out waiting for mysql to stop after %d seconds", timeoutInSec)
		} else {
			logger.Info("Successfully stopped mysql process", lager.Data{"url": statusUrl})
		}
	}

	return nil
}
