package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGenerated(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	chain := RequestID()(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header")
	}
}

func TestRequestIDPreserved(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id != "existing-id" {
			t.Errorf("expected existing-id, got %s", id)
		}
		w.WriteHeader(http.StatusOK)
	})

	chain := RequestID()(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "existing-id" {
		t.Errorf("expected existing-id in response, got %s", rr.Header().Get("X-Request-ID"))
	}
}

func TestGetRequestIDNoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	id := GetRequestID(req.Context())
	if id != "" {
		t.Errorf("expected empty string, got %s", id)
	}
}
