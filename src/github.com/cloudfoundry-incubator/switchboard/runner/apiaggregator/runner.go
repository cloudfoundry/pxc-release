package apiaggregator

import (
	"net/http"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

func NewRunner(address string, handler http.Handler) ifrit.Runner {
	return http_server.New(address, handler)
}
