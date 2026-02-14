package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestRouterExactMatch(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "exact",
			Host:     "example.com",
			Upstream: "backend-a",
			Paths:    []config.PathRule{{Path: "/api/v1", Type: "exact"}},
		},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/v1", nil)
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if upstream != "backend-a" {
		t.Errorf("expected backend-a, got %s", upstream)
	}
}

func TestRouterPrefixMatch(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "prefix",
			Host:     "",
			Upstream: "backend-b",
			Paths:    []config.PathRule{{Path: "/api", Type: "prefix"}},
		},
	})

	req := httptest.NewRequest("GET", "http://localhost/api/v1/users", nil)
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if upstream != "backend-b" {
		t.Errorf("expected backend-b, got %s", upstream)
	}
}

func TestRouterNoMatch(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "api",
			Host:     "example.com",
			Upstream: "backend",
			Paths:    []config.PathRule{{Path: "/api", Type: "prefix"}},
		},
	})

	req := httptest.NewRequest("GET", "http://other.com/web", nil)
	_, ok := router.Match(req)
	if ok {
		t.Error("expected no match")
	}
}

func TestRouterLongestPrefixWins(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "short",
			Host:     "",
			Upstream: "short-backend",
			Paths:    []config.PathRule{{Path: "/api", Type: "prefix"}},
		},
		{
			Name:     "long",
			Host:     "",
			Upstream: "long-backend",
			Paths:    []config.PathRule{{Path: "/api/v2", Type: "prefix"}},
		},
	})

	req := httptest.NewRequest("GET", "http://localhost/api/v2/users", nil)
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if upstream != "long-backend" {
		t.Errorf("expected long-backend, got %s", upstream)
	}
}

func TestRouterWildcardHost(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "wildcard",
			Host:     "",
			Upstream: "default",
			Paths:    []config.PathRule{{Path: "/", Type: "prefix"}},
		},
	})

	req := httptest.NewRequest("GET", "http://anything.com/whatever", nil)
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match with empty host (wildcard)")
	}
	if upstream != "default" {
		t.Errorf("expected default, got %s", upstream)
	}
}

func TestRouterExactMatchOverPrefix(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "prefix-route",
			Host:     "example.com",
			Upstream: "prefix-backend",
			Paths:    []config.PathRule{{Path: "/api", Type: "prefix"}},
		},
		{
			Name:     "exact-route",
			Host:     "example.com",
			Upstream: "exact-backend",
			Paths:    []config.PathRule{{Path: "/api/v1", Type: "exact"}},
		},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/v1", nil)
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if upstream != "exact-backend" {
		t.Errorf("expected exact-backend (exact over prefix), got %s", upstream)
	}
}

func TestRouterReload(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "old",
			Host:     "",
			Upstream: "old-backend",
			Paths:    []config.PathRule{{Path: "/", Type: "prefix"}},
		},
	})

	req := httptest.NewRequest("GET", "http://localhost/test", nil)
	upstream, _ := router.Match(req)
	if upstream != "old-backend" {
		t.Fatalf("expected old-backend, got %s", upstream)
	}

	// Reload with new routes
	router.Reload([]config.Route{
		{
			Name:     "new",
			Host:     "",
			Upstream: "new-backend",
			Paths:    []config.PathRule{{Path: "/", Type: "prefix"}},
		},
	})

	upstream, _ = router.Match(req)
	if upstream != "new-backend" {
		t.Errorf("expected new-backend after reload, got %s", upstream)
	}
}

func TestRouterHostWithPort(t *testing.T) {
	router := NewRouter()
	router.Reload([]config.Route{
		{
			Name:     "api",
			Host:     "example.com",
			Upstream: "backend",
			Paths:    []config.PathRule{{Path: "/", Type: "prefix"}},
		},
	})

	// Request with port in host
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Host = "example.com:8080"
	upstream, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match with host:port")
	}
	if upstream != "backend" {
		t.Errorf("expected backend, got %s", upstream)
	}
}
