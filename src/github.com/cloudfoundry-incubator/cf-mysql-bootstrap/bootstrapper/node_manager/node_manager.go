package node_manager

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . NodeManager

const (
	PollingIntervalInSec = 5
	ShutDownTimeout      = 630
)

var GetShutDownTimeout = func() int {
	return ShutDownTimeout
}

var skipCAVerification = false // Only unit tests should set this to True.

type NodeManager interface {
	VerifyClusterIsUnhealthy() (bool, error)
	VerifyAllNodesAreReachable() error
	StopAllNodes() error
	GetSequenceNumbers() (map[string]int, error)
	BootstrapNode(baseURL string) error
	JoinNode(baseURL string) error
	FindUnhealthyNode() (string, error)
	StopNode(string) error
}

type nodeManager struct {
	rootConfig *config.Config
	clock      clock.Clock
}

func New(rootConfig *config.Config, clock clock.Clock) NodeManager {
	return &nodeManager{
		rootConfig: rootConfig,
		clock:      clock,
	}
}

func (nm *nodeManager) FindUnhealthyNode() (string, error) {
	unhealthyIndex := -1
	var unhealthyURL string
	for index, url := range nm.rootConfig.HealthcheckURLs {
		responseBody, err := nm.sendGetRequest(url)
		if err != nil && !strings.Contains(responseBody, "arbitrator") {
			if unhealthyIndex != -1 {
				return "", errors.New("Found more than one unhealthy node")
			}
			unhealthyIndex = index
			unhealthyURL = url
		}
	}

	if unhealthyIndex == -1 {
		return "", errors.New("Found no unhealthy nodes")
	} else {
		return unhealthyURL, nil
	}
}

func (nm *nodeManager) VerifyClusterIsUnhealthy() (bool, error) {
	syncedNodes, allNodes := nm.determineNodeCount()

	if syncedNodes == allNodes {
		err := fmt.Errorf("All nodes are synced, %s not required.", nm.rootConfig.RepairMode)
		nm.rootConfig.Logger.Error("Action not required", err)
		return false, nil
	}

	return true, nm.validateNodeCountForRepairMode(syncedNodes, allNodes, nm.rootConfig.RepairMode)
}

func (nm *nodeManager) determineNodeCount() (int, int) {
	allNodes := len(nm.rootConfig.HealthcheckURLs)
	syncedNodes := 0

	for _, url := range nm.rootConfig.HealthcheckURLs {
		responseBody, err := nm.sendGetRequest(url)
		nm.rootConfig.Logger.Info("Received response from node", lager.Data{
			"url":          url,
			"responseBody": responseBody,
		})
		if err == nil {
			syncedNodes++
		} else if strings.Contains(responseBody, "arbitrator") {
			allNodes--
		}
	}

	return syncedNodes, allNodes
}

func (nm *nodeManager) validateNodeCountForRepairMode(syncedNodes, allNodes int, repairMode string) error {
	if repairMode == "rejoin-unsafe" {
		if syncedNodes < (allNodes - 1) {
			err := errors.New("More than one node is unhealthy, cannot execute rejoin-unsafe.")
			nm.rootConfig.Logger.Error("Action cannot be performed", err)
			return err
		}
	} else {
		if syncedNodes > 0 && syncedNodes != allNodes {
			err := errors.New("Cluster healthy but one or more nodes are failing. Bootstrap not required.")
			nm.rootConfig.Logger.Error("Bootstrap not required", err)
			return err
		}
	}
	return nil
}

func (nm *nodeManager) VerifyAllNodesAreReachable() error {
	for _, url := range nm.rootConfig.HealthcheckURLs {
		statusMysqlUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.MysqlStatus)
		_, err := nm.sendGetRequest(statusMysqlUrl)
		if err != nil {
			err = fmt.Errorf("Could not reach node: %s, received: %s", url, err.Error())
			return err
		}
	}
	return nil
}

func (nm *nodeManager) StopAllNodes() error {
	for _, url := range nm.rootConfig.HealthcheckURLs {
		stopMysqlUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.ShutDownMysql)
		_, err := nm.sendPostRequest(stopMysqlUrl)
		if err != nil {
			return err
		}
	}

	return nm.waitForClusterShutdown()
}

func (nm *nodeManager) StopNode(url string) error {
	stopMysqlUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.ShutDownMysql)
	_, err := nm.sendPostRequest(stopMysqlUrl)

	if err != nil {
		return fmt.Errorf("Failed to stop node at %s, got error: %s", url, err.Error())
	}

	statusUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.MysqlStatus)
	err = nm.pollUntilResponse(statusUrl, "stopped")
	if err != nil {
		return fmt.Errorf("Failed to stop node at %s, got error: %s", url, err.Error())
	}

	return nil
}

func (nm *nodeManager) GetSequenceNumbers() (map[string]int, error) {
	sequenceNumberMap := make(map[string]int)

	for _, url := range nm.rootConfig.HealthcheckURLs {
		getSeqNumberUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.GetSeqNumber)

		responseBody, err := nm.sendGetRequest(getSeqNumberUrl)
		if err != nil {
			return nil, err
		}

		if strings.Contains(responseBody, "arbitrator") {
			sequenceNumberMap[url] = -1
		} else {
			sequenceNumber, err := strconv.Atoi(responseBody)
			if err != nil {
				return nil, fmt.Errorf("Failed to get valid sequence number from %s with %s", getSeqNumberUrl, err.Error())
			}

			nm.rootConfig.Logger.Info(fmt.Sprintf("Retrieved sequence number of %d from %s", sequenceNumber, getSeqNumberUrl), lager.Data{
				"url": getSeqNumberUrl,
			})

			sequenceNumberMap[url] = sequenceNumber
		}
	}
	return sequenceNumberMap, nil
}

func (nm *nodeManager) BootstrapNode(baseURL string) error {
	return nm.startNodeWithURL(baseURL, nm.rootConfig.StartMysqlInBootstrapMode)
}

func (nm *nodeManager) JoinNode(baseURL string) error {
	return nm.startNodeWithURL(baseURL, nm.rootConfig.StartMysqlInJoinMode)
}

func (nm *nodeManager) startNodeWithURL(baseURL string, startEndpoint string) error {
	startURL := fmt.Sprintf("%s/%s?sst=true", baseURL, startEndpoint)
	_, err := nm.sendPostRequest(startURL)
	if err != nil {
		return err
	}

	statusUrl := fmt.Sprintf("%s/%s", baseURL, nm.rootConfig.MysqlStatus)
	for {
		responseBody, err := nm.sendGetRequest(statusUrl)
		if err != nil {
			nm.rootConfig.Logger.Info("Sending status request failed", lager.Data{
				"endpoint":     statusUrl,
				"responseBody": responseBody,
			})
			return err
		}
		nm.rootConfig.Logger.Info("Received response from status endpoint", lager.Data{
			"endpoint":     statusUrl,
			"responseBody": responseBody,
		})
		if responseBody == "running" {
			break
		} else if responseBody == "failing" {
			return fmt.Errorf("Node is failing: %s", baseURL)
		}
		<-nm.clock.After(time.Duration(PollingIntervalInSec) * time.Second)
	}

	return nil
}

func (nm *nodeManager) pollUntilResponse(endpoint string, expectedResponse string) error {
	maxIterations := int(math.Ceil(float64(GetShutDownTimeout()) / float64(PollingIntervalInSec)))
	sawResponse := false
	for i := 0; i < maxIterations; i++ {
		responseBody, err := nm.sendGetRequest(endpoint)
		if err != nil {
			nm.rootConfig.Logger.Info("Sending status request failed", lager.Data{
				"endpoint":     endpoint,
				"responseBody": responseBody,
			})
			return err
		}
		nm.rootConfig.Logger.Info("Received response from status endpoint", lager.Data{
			"endpoint":     endpoint,
			"responseBody": responseBody,
		})

		if responseBody == expectedResponse {
			sawResponse = true
			break
		}
		<-nm.clock.After(time.Duration(PollingIntervalInSec) * time.Second)
	}
	if sawResponse == false {
		return fmt.Errorf("Timed out waiting for %s from mysql after %d seconds", expectedResponse, GetShutDownTimeout())
	} else {
		nm.rootConfig.Logger.Info(fmt.Sprintf("Successfully received %s response from mysql", expectedResponse), lager.Data{"url": endpoint})
		return nil
	}
}

func (nm *nodeManager) waitForClusterShutdown() error {
	shutdownClusters := make(chan error, len(nm.rootConfig.HealthcheckURLs))

	for _, url := range nm.rootConfig.HealthcheckURLs {
		statusUrl := fmt.Sprintf("%s/%s", url, nm.rootConfig.MysqlStatus)
		go func() {
			err := nm.pollUntilResponse(statusUrl, "stopped")
			shutdownClusters <- err
		}()
	}

	for _ = range nm.rootConfig.HealthcheckURLs {
		err := <-shutdownClusters
		if err != nil {
			return err
		}
	}

	nm.rootConfig.Logger.Info("Successfully stopped mysql process on all vms")
	return nil
}

func (nm *nodeManager) sendPostRequest(endpoint string) (string, error) {
	return nm.sendRequest(endpoint, "POST")
}

func (nm *nodeManager) sendGetRequest(endpoint string) (string, error) {
	return nm.sendRequest(endpoint, "GET")
}

func (nm *nodeManager) sendRequest(endpoint string, method string) (string, error) {
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(nm.rootConfig.Username, nm.rootConfig.Password)

	// create new http client
	httpClient := &http.Client{}

	if nm.rootConfig.BackendTLS.Enabled {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(nm.rootConfig.BackendTLS.CA)); !ok {
			// TODO: should we handle the failure parsing a CA?
		}

		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            certPool,
				ServerName:         nm.rootConfig.BackendTLS.ServerName,
				InsecureSkipVerify: nm.rootConfig.BackendTLS.InsecureSkipVerify,
			},
		}
	}

	resp, err := httpClient.Do(req)
	responseBody := ""
	if err != nil {
		return responseBody, fmt.Errorf("Failed to %s: %s", endpoint, err.Error())
	}

	if resp.Body != nil {
		responseBytes, _ := ioutil.ReadAll(resp.Body)
		responseBody = string(responseBytes)
	}

	if resp.StatusCode != http.StatusOK {
		return responseBody, fmt.Errorf("Non 200 response from %s: %s", endpoint, responseBody)
	}

	nm.rootConfig.Logger.Info(fmt.Sprintf("Successfully sent %s request to URL", endpoint))

	return responseBody, nil
}
