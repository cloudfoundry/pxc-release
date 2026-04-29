// Package galera_init_client is an HTTP client for the galera-init status server (POST /start,
// POST /stop, GET / and GET /status, GET /v1/moniteq) used in place of direct monit control.
package galera_init_client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Monit-compatible status value returned by GET /v1/moniteq when mysqld is fully stopped.
const monitStringStopped = "stopped"

const readinessPhaseFailed = "failed"

// Client talks to a galera-init HTTP server.
type Client struct {
	baseURL          *url.URL
	operationTimeout time.Duration
	pollInterval     time.Duration
}

// NewClient parses baseURL (e.g. http://127.0.0.1:8114) and uses operationTimeout for Start/Stop
// total wait, polling readiness every second.
func NewClient(baseURL string, operationTimeout time.Duration) (*Client, error) {
	if operationTimeout <= 0 {
		return nil, errors.New("operationTimeout must be positive")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		return nil, errors.New("baseURL must include a scheme, e.g. http://127.0.0.1:8114")
	}
	if u.Host == "" {
		return nil, errors.New("baseURL must include a host")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	return &Client{
		baseURL:          u,
		operationTimeout: operationTimeout,
		pollInterval:     time.Second,
	}, nil
}

// NewClientForAddress builds base URL as http://<host:port> when the scheme is omitted.
func NewClientForAddress(address string, operationTimeout time.Duration) (*Client, error) {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return NewClient(address, operationTimeout)
	}
	return NewClient("http://"+address, operationTimeout)
}

type ackResponse struct {
	OK    bool         `json:"ok"`
	Error *errorObject `json:"error,omitempty"`
}

type errorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type readinessResponse struct {
	Ready bool         `json:"ready"`
	Phase string       `json:"phase"`
	Error *errorObject `json:"error,omitempty"`
}

// Start sends POST /start, then blocks until GET / reports ready (HTTP 200, ready=true) or failure.
// serviceName is ignored; it exists for the NodeManager [MonitClient] interface.
func (c *Client) Start(_ string) error {
	if err := c.postJSON("/start", "start"); err != nil {
		return err
	}
	if err := c.waitUntilReady("start"); err != nil {
		return errors.Wrap(err, "wait for galera-init readiness after start")
	}
	return nil
}

// Stop sends POST /stop, then blocks until GET /v1/moniteq body is the stopped string.
func (c *Client) Stop(_ string) error {
	if err := c.postJSON("/stop", "stop"); err != nil {
		return err
	}
	if err := c.waitUntilStatus(monitStringStopped, "stop"); err != nil {
		return errors.Wrap(err, "wait for galera-init after stop")
	}
	return nil
}

// Status returns the plain text from GET /v1/moniteq (monit-style status).
func (c *Client) Status(_ string) (string, error) {
	b, err := c.getPath("/v1/moniteq")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// buildURL resolves path relative to the client base, e.g. "start" -> {base}/start, "" -> {base}/.
func (c *Client) buildURL(relpath string) string {
	relpath = strings.TrimSpace(relpath)
	if relpath == "" || relpath == "/" {
		u2 := *c.baseURL
		u2.Path = "/"
		u2.RawPath = ""
		return u2.String()
	}
	relpath = strings.Trim(relpath, "/")
	parts := strings.Split(relpath, "/")
	var segs []string
	for _, p := range parts {
		if p != "" {
			segs = append(segs, p)
		}
	}
	if len(segs) == 0 {
		return c.buildURL("/")
	}
	return c.baseURL.JoinPath(segs...).String()
}

func (c *Client) postJSON(relativePath, op string) error {
	u := c.buildURL(relativePath)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	b, sc, err := c.doRequest(req, op)
	if err != nil {
		return err
	}
	if sc < 200 || sc > 299 {
		return errors.Errorf("galera-init %s: HTTP %d, body: %q", op, sc, string(b))
	}
	var ack ackResponse
	if err := json.Unmarshal(b, &ack); err != nil {
		return errors.Wrapf(err, "decode %s response", op)
	}
	if !ack.OK {
		if ack.Error != nil && ack.Error.Message != "" {
			return errors.Errorf("galera-init %s: %s", op, ack.Error.Message)
		}
		return errors.Errorf("galera-init %s failed (ok=false)", op)
	}
	return nil
}

func (c *Client) getPath(relativePath string) ([]byte, error) {
	u := c.buildURL(relativePath)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	b, sc, err := c.doRequest(req, "get")
	if err != nil {
		return nil, err
	}
	if sc < 200 || sc > 299 {
		return b, errors.Errorf("galera-init: HTTP %d, body: %q", sc, string(b))
	}
	return b, nil
}

func (c *Client) getReadiness() ([]byte, int, error) {
	u := c.buildURL("/")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	b, sc, err := c.doRequest(req, "get-readiness")
	if err != nil {
		return nil, 0, err
	}
	return b, sc, nil
}

func (c *Client) waitUntilReady(ctxTag string) error {
	deadline := time.NewTimer(c.operationTimeout)
	defer deadline.Stop()
	tick := time.NewTimer(0)
	defer tick.Stop()

	for {
		select {
		case <-deadline.C:
			return errors.Errorf("timed out waiting for galera-init readiness after %s", ctxTag)
		case <-tick.C:
		}

		body, sc, err := c.getReadiness()
		if err != nil {
			return err
		}

		if sc == http.StatusServiceUnavailable {
			if failErr := c.readinessFailureIfAny(body); failErr != nil {
				return failErr
			}
		} else if sc == http.StatusOK {
			// Match legacy: non-JSON 200 is treated as ready
			if !json.Valid(body) {
				return nil
			}
			var r readinessResponse
			if err := json.Unmarshal(body, &r); err != nil {
				return errors.Wrap(err, "decode readiness after start")
			}
			if r.Error != nil && r.Error.Message != "" {
				return errors.New(r.Error.Message)
			}
			if r.Phase == readinessPhaseFailed {
				if r.Error != nil && r.Error.Message != "" {
					return errors.New(r.Error.Message)
				}
				return errors.New("galera-init in failed state")
			}
			if r.Ready {
				return nil
			}
		} else {
			if failErr := c.readinessFailureIfAny(body); failErr != nil {
				return failErr
			}
			return errors.Errorf("unexpected galera-init readiness: HTTP %d, body: %q", sc, string(body))
		}

		tick.Reset(c.pollInterval)
	}
}

func (c *Client) readinessFailureIfAny(body []byte) error {
	if !json.Valid(body) {
		return nil
	}
	var r readinessResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	if r.Phase == readinessPhaseFailed {
		if r.Error != nil && r.Error.Message != "" {
			return errors.New(r.Error.Message)
		}
		return errors.New("galera-init in failed state")
	}
	if r.Error != nil && r.Error.Message != "" {
		if r.Ready {
			return nil
		}
		return errors.New(r.Error.Message)
	}
	return nil
}

func (c *Client) waitUntilStatus(want, ctxTag string) error {
	deadline := time.NewTimer(c.operationTimeout)
	defer deadline.Stop()
	tick := time.NewTimer(0)
	defer tick.Stop()

	for {
		select {
		case <-deadline.C:
			return errors.Errorf("timed out waiting for galera-init status %q after %s", want, ctxTag)
		case <-tick.C:
		}

		s, err := c.Status("")
		if err == nil && strings.TrimSpace(s) == want {
			return nil
		}
		tick.Reset(c.pollInterval)
	}
}

func (c *Client) doRequest(req *http.Request, _ string) (body []byte, status int, err error) {
	hc := &http.Client{Timeout: c.requestHTTPTimeout()}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, resp.StatusCode, rerr
	}
	return b, resp.StatusCode, nil
}

// Single-request timeout: cap so very large operationTimeout does not make one request hang the whole time.
func (c *Client) requestHTTPTimeout() time.Duration {
	const perRequestMax = 30 * time.Second
	if c.operationTimeout < perRequestMax {
		return c.operationTimeout
	}
	return perRequestMax
}
