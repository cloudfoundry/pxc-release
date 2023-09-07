package domain

import (
	"log/slog"

	"github.com/cloudfoundry-incubator/switchboard/config"
)

var BackendProvider = NewBackend

func NewBackends(backendConfigs []config.Backend, logger *slog.Logger) (backends []*Backend) {
	for _, bc := range backendConfigs {
		backends = append(backends, BackendProvider(
			bc.Name,
			bc.Host,
			bc.Port,
			bc.StatusPort,
			bc.StatusEndpoint,
			logger,
		))
	}

	return backends
}
