package middleware_test

import (
	"log/slog"
	"net/http"

	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/switchboard/api/apifakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/switchboard/api/middleware"
	"github.com/cloudfoundry-incubator/switchboard/api/middleware/fakes"
)

var _ = Describe("Logger", func() {

	var (
		dummyRequest       *http.Request
		err                error
		fakeResponseWriter http.ResponseWriter
		fakeHandler        *fakes.FakeHandler
		logger             *slog.Logger
		logBuffer          *gbytes.Buffer
		routePrefix        string
	)

	const fakePassword = "fakePassword"

	BeforeEach(func() {
		routePrefix = "/v0"
		dummyRequest, err = http.NewRequest("GET", "/v0/backends", nil)
		Expect(err).NotTo(HaveOccurred())
		dummyRequest.Header.Add("Authorization", fakePassword)

		fakeResponseWriter = new(apifakes.FakeResponseWriter)
		fakeHandler = new(fakes.FakeHandler)

		logBuffer = gbytes.NewBuffer()
		logger = slog.New(slog.NewJSONHandler(logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	It("should log requests that are prefixed with routePrefix", func() {
		loggerMiddleware := middleware.NewLogger(logger, routePrefix)
		loggerHandler := loggerMiddleware.Wrap(fakeHandler)

		loggerHandler.ServeHTTP(fakeResponseWriter, dummyRequest)

		Expect(logBuffer).To(gbytes.Say("request"))
		Expect(logBuffer).To(gbytes.Say("response"))
	})

	It("should not log credentials", func() {
		loggerMiddleware := middleware.NewLogger(logger, routePrefix)
		loggerHandler := loggerMiddleware.Wrap(fakeHandler)

		loggerHandler.ServeHTTP(fakeResponseWriter, dummyRequest)

		Expect(logBuffer).ToNot(gbytes.Say(fakePassword))
	})

	It("should call next handler", func() {
		loggerMiddleware := middleware.NewLogger(logger, routePrefix)
		loggerHandler := loggerMiddleware.Wrap(fakeHandler)

		loggerHandler.ServeHTTP(fakeResponseWriter, dummyRequest)

		Expect(fakeHandler.ServeHTTPCallCount()).To(Equal(1))
		arg0, arg1 := fakeHandler.ServeHTTPArgsForCall(0)
		Expect(arg0).ToNot(BeNil())
		Expect(arg1).To(Equal(dummyRequest))
	})
})
