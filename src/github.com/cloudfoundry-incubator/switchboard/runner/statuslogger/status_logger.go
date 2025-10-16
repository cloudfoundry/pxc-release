package statuslogger

import (
	"os"
	"sync"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
)

type StatusLogger struct {
	logger      lager.Logger
	backends    []*domain.Backend
	monitor     *monitor.ClusterMonitor
	interval    time.Duration
	backendChan chan *domain.Backend

	// Protected by mutex
	activeBackend    *domain.Backend
	lastFailoverTime time.Time
	lastFailoverFrom *domain.Backend
	failoverCount    int
	backendMutex     sync.RWMutex
}

func NewStatusLogger(
	backends []*domain.Backend,
	monitor *monitor.ClusterMonitor,
	interval time.Duration,
	logger lager.Logger,
) *StatusLogger {
	return &StatusLogger{
		logger:      logger,
		backends:    backends,
		monitor:     monitor,
		interval:    interval,
		backendChan: make(chan *domain.Backend, 1),
	}
}

// ActiveBackendChan returns the channel for receiving active backend updates.
// This is primarily exposed for testing purposes.
func (s *StatusLogger) ActiveBackendChan() chan *domain.Backend {
	return s.backendChan
}

func (s *StatusLogger) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	s.logger.Info("Status logger starting", lager.Data{
		"interval":      s.interval.String(),
		"backend_count": len(s.backends),
	})

	done := make(chan struct{})
	shutdown := make(chan struct{})

	// Register to receive active backend notifications
	s.monitor.RegisterBackendSubscriber(s.backendChan)

	// Listen for active backend changes and track failovers
	go func() {
		defer close(done)
		for {
			select {
			case newActiveBackend := <-s.backendChan:
				s.backendMutex.Lock()

				// Track failover if backend changed
				if s.activeBackend != newActiveBackend && s.activeBackend != nil {
					now := time.Now()
					s.lastFailoverTime = now
					s.lastFailoverFrom = s.activeBackend
					s.failoverCount++
				}

				s.activeBackend = newActiveBackend
				s.backendMutex.Unlock()
			case <-shutdown:
				return
			}
		}
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	close(ready)

	for {
		select {
		case <-signals:
			s.logger.Info("Status logger received shutdown signal")
			close(shutdown) // Signal goroutine to exit
			<-done          // Wait for goroutine to exit
			return nil
		case <-ticker.C:
			s.logStatus()
		}
	}
}

var _ ifrit.Runner = (*StatusLogger)(nil)

func (s *StatusLogger) logStatus() {
	// Collect connection counts and health status
	var (
		totalConnections  uint
		healthyCount      int
		unhealthyBackends []string
	)

	for _, backend := range s.backends {
		backendJSON := backend.AsJSON()
		totalConnections += backendJSON.CurrentSessionCount

		if backendJSON.Healthy {
			healthyCount++
		} else {
			unhealthyBackends = append(unhealthyBackends, backendJSON.Name)
		}
	}

	// Build log data with required fields
	logData := lager.Data{
		"total_connections": totalConnections,
		"healthy_backends":  healthyCount,
		"total_backends":    len(s.backends),
	}

	// Add active backend info
	s.backendMutex.RLock()
	if s.activeBackend != nil {
		json := s.activeBackend.AsJSON()
		logData["active_backend"] = json.Name
	} else {
		logData["active_backend"] = "none"
	}

	// Add failover info if one has occurred
	if !s.lastFailoverTime.IsZero() {
		logData["last_failover_at"] = s.lastFailoverTime.Format(time.RFC3339)
		logData["total_failovers"] = s.failoverCount
		if s.lastFailoverFrom != nil {
			logData["last_failover_from"] = s.lastFailoverFrom.AsJSON().Name
		}
	}
	s.backendMutex.RUnlock()

	// Add unhealthy backends if any (elide when empty)
	if len(unhealthyBackends) > 0 {
		logData["unhealthy_backends"] = unhealthyBackends
	}

	// Log status update
	s.logger.Info("Status update", logData)
}
