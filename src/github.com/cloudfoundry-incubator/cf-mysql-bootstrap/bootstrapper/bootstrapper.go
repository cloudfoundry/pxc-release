package bootstrapper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
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

	logger.Info("Successfully stopped mysql process on all vms")
	sequenceNumberMap := make(map[string]int)
	for _, url := range b.rootConfig.HealthcheckURLs {
		getSeqNumberUrl := fmt.Sprintf("%s/%s", url, b.rootConfig.GetSeqNumber)
		resp, err := http.Get(getSeqNumberUrl)
		if err != nil {
			return fmt.Errorf("Failed to get sequence number from mysql: %s", err.Error())
		}

		responseBody := ""
		if resp.Body != nil {
			responseBytes, _ := ioutil.ReadAll(resp.Body)
			responseBody = string(responseBytes)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Non 200 response for fetching sequence number from mysql at %s: %s", getSeqNumberUrl, responseBody)
		}

		logger.Info("resp body", lager.Data{
			"seqno response": responseBody})
		sequenceNumber, err := strconv.Atoi(responseBody)

		if err != nil {
			return fmt.Errorf("Failed to get valid sequence number from %s with %s", getSeqNumberUrl, err.Error())
		}

		sequenceNumberMap[url] = sequenceNumber

		logger.Info(fmt.Sprintf("Successfully sent request to URL to fetch sequence number: %s", getSeqNumberUrl))
	}

	bootstrapNode, joinNodes := largestSequenceNumber(sequenceNumberMap)
	bootstrapReqURL := fmt.Sprintf("%s/%s", bootstrapNode, b.rootConfig.StartMysqlInBootstrapMode)
	resp, err := http.Get(bootstrapReqURL)
	if err != nil {
		return fmt.Errorf("Failed to bootstrap mysql node: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		responseBody := ""
		if resp.Body != nil {
			responseBytes, _ := ioutil.ReadAll(resp.Body)
			responseBody = string(responseBytes)
		}
		return fmt.Errorf("Non 200 response from bootstrapping mysql at %s: %s", bootstrapReqURL, responseBody)
	}

	logger.Info(fmt.Sprintf("Successfully sent bootstrap request to URL: %s", bootstrapReqURL))

	statusUrl := fmt.Sprintf("%s/%s", bootstrapNode, b.rootConfig.MysqlStatus)
	runningSuccessfully := false

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

		if responseBody == "running" {
			runningSuccessfully = true
			break
		}

		<-b.clock.After(time.Duration(pollingIntervalInSec) * time.Second)
	}

	if runningSuccessfully == false {
		return fmt.Errorf("Timed out waiting for mysql to start after %d seconds", timeoutInSec)
	} else {
		logger.Info("Successfully bootstrapped mysql node", lager.Data{"url": statusUrl})
	}

	for _, joinNode := range joinNodes {
		joinReqURL := fmt.Sprintf("%s/%s", joinNode, b.rootConfig.StartMysqlInJoinMode)
		resp, err := http.Get(joinReqURL)
		if err != nil {
			return fmt.Errorf("Failed to join mysql node: %s", err.Error())
		}

		if resp.StatusCode != http.StatusOK {
			responseBody := ""
			if resp.Body != nil {
				responseBytes, _ := ioutil.ReadAll(resp.Body)
				responseBody = string(responseBytes)
			}
			return fmt.Errorf("Non 200 response from joining mysql at %s: %s", joinReqURL, responseBody)
		}

		logger.Info(fmt.Sprintf("Successfully sent join request to URL: %s", joinReqURL))
	}

	for _, url := range joinNodes {
		statusUrl := fmt.Sprintf("%s/%s", url, b.rootConfig.MysqlStatus)
		runningSuccessfully := false

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

			if responseBody == "running" {
				runningSuccessfully = true
				break
			}

			<-b.clock.After(time.Duration(pollingIntervalInSec) * time.Second)
		}

		if runningSuccessfully == false {
			return fmt.Errorf("Timed out waiting for mysql to start after %d seconds", timeoutInSec)
		} else {
			logger.Info("Successfully running mysql process", lager.Data{"url": statusUrl})
		}
	}
	logger.Info("Successfully started mysql process on all joining nodes")

	return nil
}

func largestSequenceNumber(seqMap map[string]int) (string, []string) {
	maxSeq := -1
	maxSeqURL := ""
	joinNodes := []string{}
	for url, seqno := range seqMap {
		if seqno > maxSeq {
			maxSeq = seqno
			maxSeqURL = url
		}
	}

	for url, _ := range seqMap {
		if url != maxSeqURL {
			joinNodes = append(joinNodes, url)
		}
	}

	return maxSeqURL, joinNodes
}
