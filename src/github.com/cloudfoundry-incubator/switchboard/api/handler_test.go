package api_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/switchboard/api"
	apifakes "github.com/cloudfoundry-incubator/switchboard/api/fakes"
	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	domainfakes "github.com/cloudfoundry-incubator/switchboard/domain/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Handler", func() {
	var (
		handler          http.Handler
		responseRecorder *httptest.ResponseRecorder
		cfg              config.API
	)

	JustBeforeEach(func() {
		backends := &domainfakes.FakeBackends{}
		logger := lagertest.NewTestLogger("Handler Test")

		staticDir := ""
		handler = api.NewHandler(backends, logger, cfg, staticDir)
	})

	Context("when a request panics", func() {
		var (
			realBackendsIndex func(backends domain.Backends) http.Handler
			responseWriter    *apifakes.FakeResponseWriter
			request           *http.Request
		)

		BeforeEach(func() {
			cfg = config.API{
				ForceHttps: false,
				Username:   "foo",
				Password:   "bar",
			}
			realBackendsIndex = api.BackendsIndex
			api.BackendsIndex = func(domain.Backends) http.Handler {
				return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					panic("fake request panic")
				})
			}

			responseWriter = &apifakes.FakeResponseWriter{}
			var err error
			request, err = http.NewRequest("GET", "/v0/backends", nil)
			Expect(err).NotTo(HaveOccurred())
			request.SetBasicAuth("foo", "bar")
		})

		AfterEach(func() {
			api.BackendsIndex = realBackendsIndex
		})

		It("recovers from panics and responds with an internal server error", func() {
			handler.ServeHTTP(responseWriter, request) // should not panic

			Expect(responseWriter.WriteHeaderCallCount()).To(Equal(1))
			Expect(responseWriter.WriteHeaderArgsForCall(0)).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("when request does not contain https header", func() {

		var request *http.Request

		BeforeEach(func() {
			cfg = config.API{
				ForceHttps: true,
			}
			responseRecorder = httptest.NewRecorder()
			request, _ = http.NewRequest("GET", "http://localhost/foo/bar", nil)
			request.Header.Set("X-Forwarded-Proto", "http")
		})

		It("redirects to https", func() {
			handler.ServeHTTP(responseRecorder, request)

			Expect(responseRecorder.Code).To(Equal(http.StatusFound))
			Expect(responseRecorder.HeaderMap.Get("Location")).To(Equal("https://localhost/foo/bar"))
		})
	})
})
