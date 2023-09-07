package monitor

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/switchboard/domain"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . UrlGetter
type UrlGetter interface {
	Get(url string) (*http.Response, error)
}

type BackendStatus struct {
	Index    int
	Healthy  bool
	Counters *DecisionCounters
}

func (b *BackendStatus) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("index", b.Index),
		slog.Bool("healthy", b.Healthy),
		slog.Any("counters", b.Counters),
	)
}

type ClusterMonitor struct {
	client             UrlGetter
	backends           []*domain.Backend
	logger             *slog.Logger
	healthcheckTimeout time.Duration
	backendSubscribers []chan<- *domain.Backend
	useLowestIndex     bool
	useTLSForAgent     bool
}

func NewClusterMonitor(client UrlGetter, useTLSForAgent bool, backends []*domain.Backend, healthcheckTimeout time.Duration, logger *slog.Logger, useLowestIndex bool) *ClusterMonitor {
	return &ClusterMonitor{
		client:             client,
		backends:           backends,
		logger:             logger,
		healthcheckTimeout: healthcheckTimeout,
		useLowestIndex:     useLowestIndex,
		useTLSForAgent:     useTLSForAgent,
	}
}

func (c *ClusterMonitor) Monitor(stopChan <-chan interface{}) {
	backendHealthMap := make(map[*domain.Backend]*BackendStatus)

	for _, backend := range c.backends {
		backendHealthMap[backend] = &BackendStatus{
			Index:    -1,
			Counters: c.SetupCounters(),
		}
	}

	go func() {
		var activeBackend *domain.Backend

		for {
			select {
			case <-time.After(c.healthcheckTimeout / 5):
				var wg sync.WaitGroup

				for backend, healthStatus := range backendHealthMap {
					wg.Add(1)
					go func(backend *domain.Backend, healthStatus *BackendStatus) {
						defer wg.Done()
						c.QueryBackendHealth(backend, healthStatus)
					}(backend, healthStatus)
				}

				wg.Wait()

				newActiveBackend := ChooseActiveBackend(backendHealthMap, c.useLowestIndex)

				if newActiveBackend != activeBackend {
					if newActiveBackend != nil {
						c.logger.Info("New active backend", "backend", newActiveBackend)
					}

					activeBackend = newActiveBackend
					for _, s := range c.backendSubscribers {
						s <- activeBackend
					}
				}

			case <-stopChan:
				return
			}
		}
	}()
}

func (c *ClusterMonitor) RegisterBackendSubscriber(newSubscriber chan<- *domain.Backend) {
	c.backendSubscribers = append(c.backendSubscribers, newSubscriber)
}

func (c *ClusterMonitor) SetupCounters() *DecisionCounters {
	counters := NewDecisionCounters()
	logFreq := uint64(5)

	//used to make logs less noisy
	counters.AddCondition("log", func() bool {
		return (counters.GetCount("dial") % logFreq) == 0
	})

	return counters
}

func ChooseActiveBackend(backendHealths map[*domain.Backend]*BackendStatus, useLowestIndex bool) *domain.Backend {
	var lowestIndexedHealthyBackend, highestIndexedHealthyBackend *domain.Backend
	lowestHealthyIndex := math.MaxUint32
	highestHealthyIndex := -1

	for backend, backendStatus := range backendHealths {
		if !backendStatus.Healthy {
			continue
		}
		if backendStatus.Index <= lowestHealthyIndex {
			lowestHealthyIndex = backendStatus.Index
			lowestIndexedHealthyBackend = backend
		}
		if backendStatus.Index >= highestHealthyIndex {
			highestHealthyIndex = backendStatus.Index
			highestIndexedHealthyBackend = backend
		}
	}

	if useLowestIndex {
		return lowestIndexedHealthyBackend
	} else {
		return highestIndexedHealthyBackend
	}
}

func (c *ClusterMonitor) determineStateFromBackend(backend *domain.Backend, shouldLog bool) (bool, *int) {
	urls := backend.HealthcheckUrls(c.useTLSForAgent)

	healthy := false
	var (
		index *int
		url   string
		err   error
		resp  *http.Response
	)

	for _, url = range urls {
		resp, err = c.client.Get(url)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				var v1StatusResponse struct {
					WsrepLocalIndex uint `json:"wsrep_local_index"`
					Healthy         bool `json:"healthy"`
				}

				_ = json.NewDecoder(resp.Body).Decode(&v1StatusResponse)

				healthy = v1StatusResponse.Healthy
				indexVal := int(v1StatusResponse.WsrepLocalIndex)
				index = &indexVal
			}
			break
		}
	}

	if shouldLog {
		if !healthy && err == nil {
			c.logger.Error(
				"Healthcheck failed on backend",
				"error", "Backend reported as unhealthy",
				"backend", backend,
				"endpoint", url,
				slog.Group("response", "status", resp.Status),
			)
		}

		if err != nil {
			c.logger.Error(
				"Error requesting healthcheck for backend",
				"error", err,
				"backend", backend,
				"endpoint", url,
			)
		}

		if healthy {
			c.logger.Debug("Healthcheck succeeded", "endpoint", url)
		}
	}

	return healthy, index
}

func (c *ClusterMonitor) QueryBackendHealth(backend *domain.Backend, healthMonitor *BackendStatus) {
	c.logger.Debug("Querying Backend",
		"backend", backend,
		"healthMonitor", healthMonitor,
	)
	shouldLog := healthMonitor.Counters.Should("log")
	healthMonitor.Counters.IncrementCount("dial")

	healthy, index := c.determineStateFromBackend(backend, shouldLog)

	if index != nil {
		healthMonitor.Index = *index
	}

	if healthy {
		c.logger.Debug("Querying Backend: healthy",
			"backend", backend,
			"healthMonitor", healthMonitor,
		)
		backend.SetHealthy()
		healthMonitor.Healthy = true
		healthMonitor.Counters.ResetCount("consecutiveUnhealthyChecks")
	} else {
		c.logger.Debug("Querying Backend: unhealthy",
			"backend", backend,
			"healthMonitor", healthMonitor,
		)
		backend.SetUnhealthy()
		healthMonitor.Healthy = false
		healthMonitor.Counters.IncrementCount("consecutiveUnhealthyChecks")
	}
}
