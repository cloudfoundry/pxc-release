package test_helpers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

func ActiveProxyBackend(proxyUsername, proxyPassword, proxyHost string, client *http.Client) (string, error) {
	requestURL := fmt.Sprintf("http://%s:8080/v0/cluster", proxyHost)
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("X-Forwarded-Proto", "https")
	req.SetBasicAuth(proxyUsername, proxyPassword)

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "http request (%v) failed", requestURL)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ERROR: Non-200 received from proxy. Status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, `failed to read proxy response`)
	}

	var cluster struct {
		ActiveBackend struct {
			Host string `json:"host"`
		} `json:"activeBackend"`
	}

	if err := json.Unmarshal(body, &cluster); err != nil {
		return "", errors.Wrap(err, `failed to unmarshal proxy response`)
	}

	return cluster.ActiveBackend.Host, nil
}
