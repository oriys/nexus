package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nexus/internal/ratelimit"
)

// KeyExtractor extracts the rate limit key from a request.
type KeyExtractor func(r *http.Request) string

// ClientIPKeyExtractor extracts the client IP as the rate limit key.
func ClientIPKeyExtractor(r *http.Request) string {
	return r.RemoteAddr
}

// RateLimit returns a middleware that enforces rate limiting.
func RateLimit(limiter *ratelimit.ShardedSlidingWindowLimiter, keyFunc KeyExtractor) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !limiter.Allow(key) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "rate_limit_exceeded",
					"message": "too many requests, please try again later",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
