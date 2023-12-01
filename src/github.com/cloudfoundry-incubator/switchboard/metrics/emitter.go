package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cloudfoundry-incubator/switchboard/domain"
)

type Emitter struct {
	backendSessions *prometheus.Desc
	backends        []*domain.Backend
	registry        *prometheus.Registry
}

func New(backends []*domain.Backend) *Emitter {
	e := &Emitter{
		registry: prometheus.NewRegistry(),
		backends: backends,
		backendSessions: prometheus.NewDesc(
			"backend_sessions_total",
			"Gauge of the current sessions from this proxy to a mysql backend",
			[]string{"backend"},
			nil,
		),
	}

	e.registry.MustRegister(e)
	return e
}
func (e *Emitter) Describe(desc chan<- *prometheus.Desc) {
	desc <- e.backendSessions
}

func (e *Emitter) Collect(metrics chan<- prometheus.Metric) {
	for _, b := range e.backends {
		j := b.AsJSON()
		metrics <- prometheus.MustNewConstMetric(e.backendSessions, prometheus.GaugeValue, float64(j.CurrentSessionCount), j.Name)
	}
}

func (e *Emitter) Handler() http.Handler {
	return promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{})
}

var _ prometheus.Collector = (*Emitter)(nil)
