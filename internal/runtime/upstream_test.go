package runtime

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestGraphQLUpstream_Handle(t *testing.T) {
	// Create a mock GraphQL backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("expected path /graphql, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Echo back the request for verification
		w.Write([]byte(`{"data":{"echo":"` + string(body) + `"}}`))
	}))
	defer backend.Close()

	upstream := &GraphQLUpstream{}
	route := &CompiledRoute{
		Name: "graphql-test",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
			GraphQL: &config.RouteUpstreamGraphQL{
				Endpoint: "/graphql",
			},
		},
	}
	cluster := &CompiledCluster{
		Name: "graphql-svc",
		Type: "graphql",
		Endpoints: []config.ClusterEndpoint{
			{URL: backend.URL},
		},
	}

	body := strings.NewReader(`{"query":"{ users { id name } }"}`)
	req := httptest.NewRequest("POST", "/api/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	err := upstream.Handle(w, req, route, cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, "echo") {
		t.Errorf("expected echo response, got %s", respBody)
	}
}

func TestGraphQLUpstream_DefaultEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("expected default path /graphql, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	upstream := &GraphQLUpstream{}
	route := &CompiledRoute{
		Name: "graphql-default",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
			// No GraphQL config — should use default "/graphql" endpoint
		},
	}
	cluster := &CompiledCluster{
		Name: "graphql-svc",
		Type: "graphql",
		Endpoints: []config.ClusterEndpoint{
			{URL: backend.URL},
		},
	}

	req := httptest.NewRequest("POST", "/gql", strings.NewReader(`{"query":"{ test }"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	err := upstream.Handle(w, req, route, cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGraphQLUpstream_CustomEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/gql" {
			t.Errorf("expected custom path /v2/gql, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	upstream := &GraphQLUpstream{}
	route := &CompiledRoute{
		Name: "graphql-custom",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
			GraphQL: &config.RouteUpstreamGraphQL{
				Endpoint: "/v2/gql",
			},
		},
	}
	cluster := &CompiledCluster{
		Name: "graphql-svc",
		Type: "graphql",
		Endpoints: []config.ClusterEndpoint{
			{URL: backend.URL},
		},
	}

	req := httptest.NewRequest("POST", "/api/graphql", strings.NewReader(`{"query":"{ test }"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	err := upstream.Handle(w, req, route, cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGraphQLUpstream_NoEndpoints(t *testing.T) {
	upstream := &GraphQLUpstream{}
	route := &CompiledRoute{
		Name: "graphql-no-ep",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
		},
	}
	cluster := &CompiledCluster{
		Name:      "graphql-svc",
		Type:      "graphql",
		Endpoints: []config.ClusterEndpoint{},
	}

	req := httptest.NewRequest("POST", "/graphql", nil)
	w := httptest.NewRecorder()

	err := upstream.Handle(w, req, route, cluster)
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
	if !strings.Contains(err.Error(), "no endpoints") {
		t.Errorf("expected 'no endpoints' error, got %s", err.Error())
	}
}

func TestUpstreamDispatcher_GraphQL(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer backend.Close()

	dispatcher := NewUpstreamDispatcher()
	route := &CompiledRoute{
		Name: "graphql-dispatch",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
			GraphQL: &config.RouteUpstreamGraphQL{
				Endpoint: "/graphql",
			},
		},
	}
	cluster := &CompiledCluster{
		Name: "graphql-svc",
		Type: "graphql",
		Endpoints: []config.ClusterEndpoint{
			{URL: backend.URL},
		},
	}

	req := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{"query":"{ test }"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	err := dispatcher.Dispatch(w, req, route, cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGraphQLUpstream_SetsContentType(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	upstream := &GraphQLUpstream{}
	route := &CompiledRoute{
		Name: "graphql-ct",
		Upstream: RouteUpstreamConfig{
			ClusterName: "graphql-svc",
		},
	}
	cluster := &CompiledCluster{
		Name: "graphql-svc",
		Type: "graphql",
		Endpoints: []config.ClusterEndpoint{
			{URL: backend.URL},
		},
	}

	// Request without Content-Type should get default set
	req := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{"query":"{ test }"}`))
	w := httptest.NewRecorder()

	err := upstream.Handle(w, req, route, cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
