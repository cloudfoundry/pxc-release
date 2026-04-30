package galera_init_status_server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"code.cloudfoundry.org/lager/v3"
)

// GaleraInitStatusServer runs the galera-init HTTP API on the given listener.
type GaleraInitStatusServer struct {
	listener net.Listener
	handler  http.Handler
	logger   lager.Logger
}

// NewGaleraInitStatusServer wires the listener to the provided handler. Logger is used for fatal serve errors.
func NewGaleraInitStatusServer(
	listener net.Listener,
	handler http.Handler,
	logger lager.Logger,
) *GaleraInitStatusServer {
	if handler == nil {
		panic("galera-init: NewGaleraInitStatusServer: handler is required")
	}
	if logger == nil {
		panic("galera-init: NewGaleraInitStatusServer: logger is required")
	}
	return &GaleraInitStatusServer{
		listener: listener,
		handler:  handler,
		logger:   logger,
	}
}

// Start runs the HTTP server; it does not block.
func (s *GaleraInitStatusServer) Start() error {
	// Long-lived lifecycle work can take minutes (e.g. internal stop before start, mysqld join).
	// Readiness is polled on GET. Zero read/write timeout avoids aborting POST before the handler
	// returns; ReadHeaderTimeout mitigates slowloris.
	server := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 1 * time.Minute,
		ReadTimeout:       0,
		WriteTimeout:      0,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		if err := server.Serve(s.listener); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != http.ErrServerClosed {
				s.logger.Fatal("galera-init-status-server", err, lager.Data{
					"detail": fmt.Sprint(s.listener.Addr()),
				})
			}
		}
	}()
	return nil
}
