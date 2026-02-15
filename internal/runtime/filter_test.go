package runtime

import (
	"net/http/httptest"
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestStripPrefixFilter(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		inputPath  string
		expectPath string
	}{
		{"basic strip", "/api/v1/http", "/api/v1/http/users", "/users"},
		{"strip to root", "/api", "/api", "/"},
		{"strip with trailing", "/api/v1", "/api/v1/", "/"},
		{"no match", "/other", "/api/v1/users", "/api/v1/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := newStripPrefixFilter(map[string]string{"prefix": tt.prefix})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := httptest.NewRequest("GET", tt.inputPath, nil)
			if err := f.Apply(req); err != nil {
				t.Fatalf("apply error: %v", err)
			}

			if req.URL.Path != tt.expectPath {
				t.Errorf("expected path %s, got %s", tt.expectPath, req.URL.Path)
			}
		})
	}
}

func TestStripPrefixFilter_MissingArg(t *testing.T) {
	_, err := newStripPrefixFilter(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing prefix arg")
	}
}

func TestHeaderSetFilter(t *testing.T) {
	f, err := newHeaderSetFilter(map[string]string{"key": "X-Gateway", "value": "nexus"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	if err := f.Apply(req); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	if req.Header.Get("X-Gateway") != "nexus" {
		t.Errorf("expected X-Gateway=nexus, got %s", req.Header.Get("X-Gateway"))
	}
}

func TestHeaderSetFilter_Overwrite(t *testing.T) {
	f, err := newHeaderSetFilter(map[string]string{"key": "Content-Type", "value": "application/json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Content-Type", "text/plain")

	if err := f.Apply(req); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %s", req.Header.Get("Content-Type"))
	}
}

func TestHeaderSetFilter_MissingKey(t *testing.T) {
	_, err := newHeaderSetFilter(map[string]string{"value": "test"})
	if err == nil {
		t.Fatal("expected error for missing key arg")
	}
}

func TestFilterRegistry_Compile(t *testing.T) {
	fr := NewFilterRegistry()

	// Test strip_prefix
	f, err := fr.Compile(config.RouteFilter{
		Type: "strip_prefix",
		Args: map[string]string{"prefix": "/api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/users", nil)
	if err := f.Apply(req); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if req.URL.Path != "/users" {
		t.Errorf("expected /users, got %s", req.URL.Path)
	}

	// Test header_set
	f, err = fr.Compile(config.RouteFilter{
		Type: "header_set",
		Args: map[string]string{"key": "X-Test", "value": "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req = httptest.NewRequest("GET", "/test", nil)
	if err := f.Apply(req); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if req.Header.Get("X-Test") != "hello" {
		t.Errorf("expected hello, got %s", req.Header.Get("X-Test"))
	}
}

func TestFilterRegistry_UnknownType(t *testing.T) {
	fr := NewFilterRegistry()
	_, err := fr.Compile(config.RouteFilter{Type: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown filter type")
	}
}

func TestFilterChain_Integration(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "backend", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://backend:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "filtered-route",
				Match: config.RouteMatch{
					Methods:    []string{"GET", "POST"},
					PathPrefix: "/api/v1/http/",
				},
				Filters: []config.RouteFilter{
					{Type: "strip_prefix", Args: map[string]string{"prefix": "/api/v1/http"}},
					{Type: "header_set", Args: map[string]string{"key": "x-gw", "value": "nexus"}},
				},
				Upstream: config.RouteUpstream{Cluster: "backend"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/http/users", nil)
	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}

	// Apply filters
	for _, f := range route.Filters {
		if err := f.Apply(req); err != nil {
			t.Fatalf("filter apply error: %v", err)
		}
	}

	if req.URL.Path != "/users" {
		t.Errorf("expected path /users after strip_prefix, got %s", req.URL.Path)
	}
	if req.Header.Get("x-gw") != "nexus" {
		t.Errorf("expected x-gw=nexus, got %s", req.Header.Get("x-gw"))
	}
}
