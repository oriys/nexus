package middleware

import (
	"net/http"
	"time"

	"github.com/oriys/nexus/internal/metrics"
)

// Metrics returns a middleware that records Prometheus request metrics.
func Metrics() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			metrics.RecordRequest(r.Method, r.URL.Path, sw.status, time.Since(start))
		})
	}
}
