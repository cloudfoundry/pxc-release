package middleware_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type dummyHandler struct {
	called bool
}

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.called = true
}

var _ = Describe("BasicAuth", func() {
	var (
		authBasic         middleware.BasicAuth
		recorder          *httptest.ResponseRecorder
		request           *http.Request
		next              *dummyHandler
		wrappedMiddleware http.Handler
	)

	BeforeEach(func() {
		authBasic = middleware.NewBasicAuth("admin-user", "secret-password").(middleware.BasicAuth)
		recorder = httptest.NewRecorder()
		next = &dummyHandler{}
		wrappedMiddleware = authBasic.Wrap(next)
	})

	Context("when no credentials are provided", func() {
		BeforeEach(func() {
			var err error
			request, err = http.NewRequest("GET", "/stop_mysql", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("denies access and does not call next handler", func() {
			wrappedMiddleware.ServeHTTP(recorder, request)

			Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
			Expect(recorder.Header().Get("WWW-Authenticate")).To(ContainSubstring(`Basic realm="Authorization Required"`))
			Expect(next.called).To(BeFalse())
		})
	})

	Context("when valid credentials are provided", func() {
		BeforeEach(func() {
			var err error
			request, err = http.NewRequest("GET", "/stop_mysql", nil)
			Expect(err).NotTo(HaveOccurred())
			request.SetBasicAuth("admin-user", "secret-password")
		})

		It("allows access and calls the next handler", func() {
			wrappedMiddleware.ServeHTTP(recorder, request)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(next.called).To(BeTrue())
		})
	})

	Context("when invalid credentials are provided", func() {
		DescribeTable("denying access across different permutations",
			func(user, pass string) {
				var err error
				request, err = http.NewRequest("GET", "/stop_mysql", nil)
				Expect(err).NotTo(HaveOccurred())
				request.SetBasicAuth(user, pass)

				wrappedMiddleware.ServeHTTP(recorder, request)

				Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(next.called).To(BeFalse())
			},
			Entry("wrong username, correct password", "wrong-user", "secret-password"),
			Entry("correct username, wrong password", "admin-user", "wrong-password"),
			Entry("wrong username, wrong password", "wrong-user", "wrong-password"),
			Entry("empty username, empty password", "", ""),
			Entry("extremely long username, correct password", "admin-user-very-long-garbage-string-to-check-handling", "secret-password"),
			Entry("correct username, extremely long password", "admin-user", "secret-password-very-long-garbage-string-to-check-handling"),
		)
	})
})
