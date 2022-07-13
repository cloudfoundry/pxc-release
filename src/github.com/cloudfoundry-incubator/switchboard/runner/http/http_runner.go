package http

import (
	"crypto/tls"
	"net/http"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

func NewHTTPRunner(address string, handler http.Handler) ifrit.Runner {
	return http_server.New(address, handler)
}

func NewHTTPRunnerWithTLS(address string, handler http.Handler, tlsConfig *tls.Config) ifrit.Runner {
	return http_server.NewTLSServer(address, handler, tlsConfig)
}
