package test_helpers

import (
	"io"
	"net/http"

	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/fakes"
)

type EndpointHandler struct {
	handlers map[string]*fakes.FakeHandler
}

func NewEndpointHandler() *EndpointHandler {
	return &EndpointHandler{
		handlers: map[string]*fakes.FakeHandler{},
	}
}

func (h *EndpointHandler) StubEndpoint(endpoint string, handler *fakes.FakeHandler) {
	h.handlers[endpoint] = handler
}

func (h *EndpointHandler) StubEndpointWithStatus(endpoint string, statusCode int, responseText ...string) {
	fakeHandler := &fakes.FakeHandler{}
	fakeHandler.ServeHTTPStub = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(statusCode)
		if len(responseText) > 0 {
			_, _ = io.WriteString(w, responseText[0])
		}
	}
	h.handlers[endpoint] = fakeHandler
}

func (h *EndpointHandler) GetFakeHandler(endpoint string) *fakes.FakeHandler {
	return h.handlers[endpoint]
}

func (h *EndpointHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	endpoint := req.URL.Path
	handler, hasKey := h.handlers[endpoint]
	if hasKey {
		handler.ServeHTTP(w, req)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}
