package middleware

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BasicAuth Internal secureCompare calls", func() {
	It("always calls secureCompare for both username and password to prevent short-circuiting", func() {
		originalSecureCompare := secureCompare
		defer func() { secureCompare = originalSecureCompare }()

		callCount := 0
		secureCompare = func(a, b string) bool {
			callCount++
			return originalSecureCompare(a, b)
		}

		authBasic := NewBasicAuth("admin-user", "secret-password").(BasicAuth)
		recorder := httptest.NewRecorder()
		request, _ := http.NewRequest("GET", "/v0/backends", nil)
		request.SetBasicAuth("wrong-user", "secret-password")

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		wrappedMiddleware := authBasic.Wrap(next)

		wrappedMiddleware.ServeHTTP(recorder, request)

		Expect(callCount).To(Equal(2))
	})

	It("always hashes compared inputs to SHA-256 to equalize lengths passed to ConstantTimeCompare", func() {
		originalConstantTimeCompare := constantTimeCompare
		defer func() { constantTimeCompare = originalConstantTimeCompare }()

		var capturedX, capturedY []byte
		constantTimeCompare = func(x, y []byte) int {
			capturedX = x
			capturedY = y
			return originalConstantTimeCompare(x, y)
		}

		_ = secureCompare("short", "extremely-long-string-value-to-test-length-leak-prevention")

		Expect(len(capturedX)).To(Equal(32))
		Expect(len(capturedY)).To(Equal(32))

		expectedX := sha256.Sum256([]byte("short"))
		expectedY := sha256.Sum256([]byte("extremely-long-string-value-to-test-length-leak-prevention"))

		Expect(capturedX).To(Equal(expectedX[:]))
		Expect(capturedY).To(Equal(expectedY[:]))
	})
})
