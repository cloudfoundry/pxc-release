package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

type BasicAuth struct {
	Username, Password string
}

func NewBasicAuth(username, password string) Middleware {
	return BasicAuth{
		Username: username,
		Password: password,
	}
}

func (b BasicAuth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		username, password, ok := req.BasicAuth()
		usernameMatch := secureCompare(username, b.Username)
		passwordMatch := secureCompare(password, b.Password)
		if ok && usernameMatch && passwordMatch {
			next.ServeHTTP(rw, req)
		} else {
			rw.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
			http.Error(rw, "Not Authorized", http.StatusUnauthorized)
		}
	})
}

var constantTimeCompare = subtle.ConstantTimeCompare

// var only so tests can spy; do not reassign elsewhere
var secureCompare = func(a, b string) bool {
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))
	return constantTimeCompare(aHash[:], bHash[:]) == 1
}
