package test_helpers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
)

func ActiveProxyBackend(proxyUsername, proxyPassword, proxyHost string) (string, error) {
	requestURL := fmt.Sprintf("http://%s:8080/v0/cluster", proxyHost)
	isEnabled, err := IsTLSEnabled("/instance_groups/name=proxy/jobs/name=proxy/properties/api_tls?/enabled")
	if err != nil {
		return "", err
	}
	if isEnabled {
		requestURL = fmt.Sprintf("https://%s:8080/v0/cluster", proxyHost)
	}

	resp, err := doGet(requestURL, proxyUsername, proxyPassword)
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

func doGet(url string, proxyUsername string, proxyPassword string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Forwarded-Proto", "https")
	req.SetBasicAuth(proxyUsername, proxyPassword)

	resp, err := HttpClient.Do(req)
	if err != nil {
		return resp, errors.Wrapf(err, "http request (%v) failed", url)
	}
	return resp, err
}

func IsTLSEnabled(path string) (bool, error) {
	cmd := exec.Command("bash", "-c",
		"bosh manifest | bosh interpolate - --path="+path)
	cmd.Stderr = ginkgo.GinkgoWriter
	out, err := cmd.Output()
	isEnabled := strings.TrimSpace(string(out)) == "true"
	if err != nil {
		return isEnabled, err
	}
	return isEnabled, nil
}
