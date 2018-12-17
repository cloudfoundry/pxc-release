package monit_client

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type MonitClient struct {
	URL      *url.URL
	User     string
	Password string
	Timeout  time.Duration
}

func NewClient(address, user, password string, timeout time.Duration) *MonitClient {
	return &MonitClient{
		URL: &url.URL{
			Scheme: "http",
			Host:   address,
		},
		User:     user,
		Password: password,
		Timeout:  timeout,
	}
}

func (c *MonitClient) Start(processName string) error {
	if _, err := c.do(http.MethodPost, "/"+processName, "action=start"); err != nil {
		return errors.Wrap(err, "failed to make start request for "+processName)
	}

	if err := c.waitForStatus(processName, ServiceRunning); err != nil {
		return errors.Wrapf(err, "timed out waiting for %s monit service to start", processName)
	}

	return nil
}

func (c *MonitClient) Stop(processName string) error {
	if _, err := c.do(http.MethodPost, "/"+processName, "action=stop"); err != nil {
		return errors.Wrap(err, "failed to make stop request for "+processName)
	}

	if err := c.waitForStatus(processName, ServiceStopped); err != nil {
		return errors.Wrapf(err, "timed out waiting for %s monit service to stop", processName)
	}

	return nil
}

func (c *MonitClient) waitForStatus(processName string, desiredServiceStatus ServiceStatus) error {
	var (
		lastServiceStatus = "unknown"
	)
	timer := time.NewTimer(c.Timeout)
	ticker := time.NewTicker(time.Second)
	defer timer.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return errors.Errorf("service status=%v", lastServiceStatus)
		case <-ticker.C:
			var err error
			lastServiceStatus, err = c.Status(processName)
			if err != nil {
				return err
			}

			if lastServiceStatus == string(desiredServiceStatus) {
				return nil
			}
		}
	}
}

func (c *MonitClient) Status(processName string) (string, error) {
	body, err := c.do(http.MethodGet, "/_status", "", url.Values{"format": []string{"xml"}})
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	monitStatus, err := ParseXML(body)
	if err != nil {
		return "", err
	}

	for _, svc := range monitStatus.Services {
		if svc.Name == processName {
			return svc.String(), nil
		}
	}

	return "", errors.New("service not found")
}

func (c *MonitClient) do(method, path, reqBody string, queryParams ...url.Values) (io.ReadCloser, error) {
	body := strings.NewReader(reqBody)

	reqURL := c.URL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequest(method, reqURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.User, c.Password)

	if len(queryParams) > 0 {
		req.URL.RawQuery = queryParams[0].Encode()
	}

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	switch response.StatusCode {
	case http.StatusOK:
		return response.Body, nil
	default:
		return nil, errors.Errorf("status code: %d", response.StatusCode)
	}
}
