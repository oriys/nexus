package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Checker provides health and readiness check endpoints.
type Checker struct {
	ready atomic.Bool
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{}
}

// SetReady marks the service as ready to accept traffic.
func (c *Checker) SetReady(ready bool) {
	c.ready.Store(ready)
}

// HealthzHandler returns a handler for the /healthz endpoint (liveness).
func (c *Checker) HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ReadyzHandler returns a handler for the /readyz endpoint (readiness).
func (c *Checker) ReadyzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if c.ready.Load() {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		}
	}
}
