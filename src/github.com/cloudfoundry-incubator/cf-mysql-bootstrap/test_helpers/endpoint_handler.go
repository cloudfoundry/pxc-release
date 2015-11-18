package test_helpers

import (
	"net/http"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
)

type EndpointHandler struct {
	Handlers map[string]*fakes.FakeHandler
}

func NewEndpointHandler() *EndpointHandler {
	return &EndpointHandler{
		Handlers: map[string]*fakes.FakeHandler{},
	}
}

func (h *EndpointHandler) StubEndpoint(endpoint string, handler ...*fakes.FakeHandler) {
	if len(handler) > 0 {
		h.Handlers[endpoint] = handler[0]
	} else {
		fakeHandler := &fakes.FakeHandler{}
		fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		h.Handlers[endpoint] = fakeHandler
	}
}

func (h *EndpointHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	endpoint := req.URL.Path
	handler, hasKey := h.Handlers[endpoint]
	if hasKey {
		handler.ServeHTTP(w, req)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}
