package test_helpers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func ActiveProxyBackend() (string, error) {
	client := &http.Client{}

	var proxyUsername = os.Getenv("PROXY_USERNAME")
	var proxyPassword = os.Getenv("PROXY_PASSWORD")

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:8080/v0/cluster", BoshEnvironment()), nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(proxyUsername, proxyPassword)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ERROR: Non-200 received from proxy. Status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var cluster struct {
		ActiveBackend struct {
			Host string `json:"host"`
		} `json:"activeBackend`
	}

	if err := json.Unmarshal(body, &cluster); err != nil {
		return "", err
	}

	return cluster.ActiveBackend.Host, nil
}
