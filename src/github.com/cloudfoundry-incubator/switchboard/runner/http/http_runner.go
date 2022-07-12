package http

import (
	"crypto/tls"
	"net/http"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

func NewRunner(address string, handler http.Handler, tlsConfig *tls.Config, enabled bool) ifrit.Runner {
	if enabled {
		return http_server.NewTLSServer(address, handler, tlsConfig)
	} else {
		return http_server.New(address, handler)
	}
}
