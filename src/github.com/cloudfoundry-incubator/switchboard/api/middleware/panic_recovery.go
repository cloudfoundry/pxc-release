package middleware

import (
	"log/slog"
	"net/http"
)

type PanicRecovery struct {
	logger *slog.Logger
}

func NewPanicRecovery(logger *slog.Logger) Middleware {
	return &PanicRecovery{logger}
}

func (p PanicRecovery) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			if panicInfo := recover(); panicInfo != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				p.logger.Error("Panic while serving request",
					"panicInfo", panicInfo,
				)
			}
		}()
		next.ServeHTTP(rw, req)
	})
}
