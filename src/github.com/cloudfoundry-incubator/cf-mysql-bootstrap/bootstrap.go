package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-golang/lager"
)

func main() {

	// call stop_mysql endpoint
	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger
	if err != nil {
		logger.Fatal("Failed to parse config", err, lager.Data{
			"config": rootConfig,
		})
	}

	for _, url := range rootConfig.HealthcheckURLs {
		stopMysqlUrl := fmt.Sprintf("%s/%s", url, rootConfig.ShutDownMysql)
		resp, err := http.Get(stopMysqlUrl)
		if err != nil {
			logger.Fatal("Failed to stop mysql ", err, lager.Data{
				"url": stopMysqlUrl,
			})
		}
		if resp.StatusCode != http.StatusOK {
			responseBody := ""
			if resp.Body != nil {
				responseBytes, _ := ioutil.ReadAll(resp.Body)
				responseBody = string(responseBytes)
			}
			err := errors.New("Non 200 response from stopping mysql")
			logger.Fatal("Received non-200 response when stopping mysql",
				err,
				lager.Data{
					"url":          stopMysqlUrl,
					"StatusCode":   resp.StatusCode,
					"responseBody": responseBody,
				})
		}

		// poll with mysql_status endpoint to check if all nodes did stop.
		//
		fmt.Printf("Successfully stopped mysql at URL: %s", stopMysqlUrl)
	}
}
