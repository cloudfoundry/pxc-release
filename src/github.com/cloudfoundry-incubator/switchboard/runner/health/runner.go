package health

import (
	"net/http"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

func NewRunner(address string) ifrit.Runner {
	return http_server.New(address, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {}))
}
