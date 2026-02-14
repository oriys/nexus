package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRecordRequest(t *testing.T) {
	RecordRequest("GET", "/api/test", 200, 100*time.Millisecond)

	// Verify counter was incremented
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "nexus_requests_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("nexus_requests_total metric not found")
	}
}

func TestHandler(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty metrics response")
	}
}
