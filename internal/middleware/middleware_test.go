package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oriys/nexus/internal/auth"
	"github.com/oriys/nexus/internal/ratelimit"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// --- Rate Limit Tests ---

func TestRateLimitMiddleware_AllowsRequests(t *testing.T) {
	limiter := ratelimit.NewLimiter(3, time.Minute)
	handler := RateLimit(limiter, ClientIPKeyExtractor)(okHandler())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimitMiddleware_DeniesOverRate(t *testing.T) {
	limiter := ratelimit.NewLimiter(3, time.Minute)
	handler := RateLimit(limiter, ClientIPKeyExtractor)(okHandler())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.2:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.2:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}

	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("expected Retry-After header to be 60, got %q", rr.Header().Get("Retry-After"))
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "rate_limit_exceeded" {
		t.Errorf("expected error 'rate_limit_exceeded', got %q", body["error"])
	}
}

// --- Auth Tests ---

func TestAuthMiddleware_ValidKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuthenticator(map[string]string{"test-key": "test-user"})
	handler := Auth(authenticator)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "test-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuthenticator(map[string]string{"test-key": "test-user"})
	handler := Auth(authenticator)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", body["error"])
	}
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuthenticator(map[string]string{"test-key": "test-user"})
	handler := Auth(authenticator)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", body["error"])
	}
}

// --- Trace Context Tests ---

func TestTraceContext_GeneratesTraceparent(t *testing.T) {
	handler := TraceContext()(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	traceparent := req.Header.Get("traceparent")
	if traceparent == "" {
		t.Fatal("expected traceparent header to be set")
	}

	if len(traceparent) != 55 {
		t.Errorf("expected traceparent length 55, got %d", len(traceparent))
	}
}

func TestTraceContext_PreservesTraceparent(t *testing.T) {
	handler := TraceContext()(okHandler())

	existing := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", existing)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	traceparent := req.Header.Get("traceparent")
	if traceparent != existing {
		t.Errorf("expected traceparent to be preserved as %q, got %q", existing, traceparent)
	}
}

func TestTraceContext_StoresTraceIDInContext(t *testing.T) {
	var capturedTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = GetTraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := TraceContext()(inner)

	existing := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", existing)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expected := "4bf92f3577b34da6a3ce929d0e0e4736"
	if capturedTraceID != expected {
		t.Errorf("expected trace ID %q, got %q", expected, capturedTraceID)
	}
}
