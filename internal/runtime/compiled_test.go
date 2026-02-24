package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestCompile_BasicHTTPRoute(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{
				Name: "user-http",
				Type: "http",
				Endpoints: []config.ClusterEndpoint{
					{URL: "http://user-svc:8080"},
				},
				LB: "round_robin",
			},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "http_passthrough",
				Match: config.RouteMatch{
					Methods:    []string{"GET", "POST"},
					PathPrefix: "/api/v1/http/",
				},
				Filters: []config.RouteFilter{
					{Type: "strip_prefix", Args: map[string]string{"prefix": "/api/v1/http"}},
					{Type: "header_set", Args: map[string]string{"key": "x-gw", "value": "nova"}},
				},
				Upstream: config.RouteUpstream{
					Cluster:   "user-http",
					TimeoutMs: 30000,
				},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	if compiled.Version != 1 {
		t.Errorf("expected version 1, got %d", compiled.Version)
	}

	if len(compiled.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(compiled.Clusters))
	}

	cluster := compiled.Clusters["user-http"]
	if cluster == nil {
		t.Fatal("cluster user-http not found")
	}
	if cluster.Type != "http" {
		t.Errorf("expected type http, got %s", cluster.Type)
	}
	if cluster.LB != "round_robin" {
		t.Errorf("expected lb round_robin, got %s", cluster.LB)
	}
}

func TestCompile_GRPCRoute(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{
				Name: "user-grpc",
				Type: "grpc",
				Endpoints: []config.ClusterEndpoint{
					{Target: "dns:///user-grpc:9090"},
				},
				GRPC: &config.ClusterGRPC{
					Authority:    "user-grpc",
					MaxRecvMsgMB: 16,
				},
			},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "http_to_grpc",
				Match: config.RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/v1/user/get",
					Headers: []config.HeaderMatch{
						{Name: "content-type", Contains: "application/json"},
					},
				},
				Upstream: config.RouteUpstream{
					Cluster: "user-grpc",
					GRPC: &config.RouteUpstreamGRPC{
						Service: "user.v1.UserService",
						Method:  "GetUser",
					},
				},
			},
		},
	}

	compiled, err := Compile(cfg, 2)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Test route matching
	req := httptest.NewRequest("POST", "/api/v1/user/get", nil)
	req.Header.Set("Content-Type", "application/json")

	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Name != "http_to_grpc" {
		t.Errorf("expected route http_to_grpc, got %s", route.Name)
	}
	if route.Upstream.ClusterName != "user-grpc" {
		t.Errorf("expected cluster user-grpc, got %s", route.Upstream.ClusterName)
	}
	if route.Upstream.GRPC == nil {
		t.Fatal("expected GRPC upstream config")
	}
	if route.Upstream.GRPC.Service != "user.v1.UserService" {
		t.Errorf("expected service user.v1.UserService, got %s", route.Upstream.GRPC.Service)
	}
}

func TestCompile_DubboRoute(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{
				Name: "order-dubbo",
				Type: "dubbo",
				Endpoints: []config.ClusterEndpoint{
					{Addr: "order-dubbo:20880"},
				},
				Dubbo: &config.ClusterDubbo{
					Application:   "nova-gw",
					Version:       "1.0.0",
					Serialization: "hessian2",
				},
			},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "http_to_dubbo",
				Match: config.RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/v1/order/create",
				},
				Upstream: config.RouteUpstream{
					Cluster: "order-dubbo",
					Dubbo: &config.RouteUpstreamDubbo{
						Interface:  "com.foo.order.OrderService",
						Method:     "CreateOrder",
						ParamTypes: []string{"com.foo.order.CreateOrderRequest"},
					},
				},
			},
		},
	}

	compiled, err := Compile(cfg, 3)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/order/create", nil)
	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Name != "http_to_dubbo" {
		t.Errorf("expected route http_to_dubbo, got %s", route.Name)
	}
	if route.Upstream.Dubbo == nil {
		t.Fatal("expected Dubbo upstream config")
	}
	if route.Upstream.Dubbo.Interface != "com.foo.order.OrderService" {
		t.Errorf("expected interface com.foo.order.OrderService, got %s", route.Upstream.Dubbo.Interface)
	}
}

func TestRouterIndex_MethodMatch(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "backend", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://localhost:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "post-only",
				Match: config.RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/submit",
				},
				Upstream: config.RouteUpstream{Cluster: "backend"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// POST should match
	req := httptest.NewRequest("POST", "/api/submit", nil)
	_, matched := compiled.Router.Match(req)
	if !matched {
		t.Error("expected POST to match")
	}

	// GET should not match
	req = httptest.NewRequest("GET", "/api/submit", nil)
	_, matched = compiled.Router.Match(req)
	if matched {
		t.Error("expected GET not to match")
	}
}

func TestRouterIndex_HeaderMatch(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "backend", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://localhost:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "json-only",
				Match: config.RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/data",
					Headers: []config.HeaderMatch{
						{Name: "content-type", Contains: "application/json"},
					},
				},
				Upstream: config.RouteUpstream{Cluster: "backend"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Request with JSON content-type should match
	req := httptest.NewRequest("POST", "/api/data", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	_, matched := compiled.Router.Match(req)
	if !matched {
		t.Error("expected JSON content-type to match")
	}

	// Request without JSON content-type should not match
	req = httptest.NewRequest("POST", "/api/data", nil)
	req.Header.Set("Content-Type", "text/plain")
	_, matched = compiled.Router.Match(req)
	if matched {
		t.Error("expected text/plain not to match")
	}
}

func TestRouterIndex_PrefixMatch(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "short", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://short:8080"}}},
			{Name: "long", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://long:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "short-prefix",
				Match:    config.RouteMatch{PathPrefix: "/api"},
				Upstream: config.RouteUpstream{Cluster: "short"},
			},
			{
				Name:     "long-prefix",
				Match:    config.RouteMatch{PathPrefix: "/api/v2"},
				Upstream: config.RouteUpstream{Cluster: "long"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// /api/v2/users should match the longer prefix
	req := httptest.NewRequest("GET", "/api/v2/users", nil)
	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Upstream.ClusterName != "long" {
		t.Errorf("expected long cluster (longest prefix), got %s", route.Upstream.ClusterName)
	}

	// /api/v1/users should match the shorter prefix
	req = httptest.NewRequest("GET", "/api/v1/users", nil)
	route, matched = compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Upstream.ClusterName != "short" {
		t.Errorf("expected short cluster, got %s", route.Upstream.ClusterName)
	}
}

func TestRouterIndex_ExactOverPrefix(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "exact-backend", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://exact:8080"}}},
			{Name: "prefix-backend", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://prefix:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "prefix-route",
				Match:    config.RouteMatch{PathPrefix: "/api"},
				Upstream: config.RouteUpstream{Cluster: "prefix-backend"},
			},
			{
				Name: "exact-route",
				Match: config.RouteMatch{
					Methods: []string{"GET"},
					Path:    "/api/v1",
				},
				Upstream: config.RouteUpstream{Cluster: "exact-backend"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Exact match should win over prefix
	req := httptest.NewRequest("GET", "/api/v1", nil)
	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Upstream.ClusterName != "exact-backend" {
		t.Errorf("expected exact-backend, got %s", route.Upstream.ClusterName)
	}
}

func TestCompiledCluster_NextEndpoint(t *testing.T) {
	cluster := &CompiledCluster{
		Name: "test",
		Type: "http",
		Endpoints: []config.ClusterEndpoint{
			{URL: "http://host1:8080"},
			{URL: "http://host2:8080"},
			{URL: "http://host3:8080"},
		},
	}

	// Test round-robin
	addrs := make([]string, 6)
	for i := 0; i < 6; i++ {
		ep, ok := cluster.NextEndpoint()
		if !ok {
			t.Fatalf("expected endpoint at iteration %d", i)
		}
		addrs[i] = EndpointAddress(ep)
	}

	// Should cycle through all 3 endpoints twice
	expected := []string{
		"http://host1:8080", "http://host2:8080", "http://host3:8080",
		"http://host1:8080", "http://host2:8080", "http://host3:8080",
	}
	for i, addr := range addrs {
		if addr != expected[i] {
			t.Errorf("iteration %d: expected %s, got %s", i, expected[i], addr)
		}
	}
}

func TestEndpointAddress(t *testing.T) {
	tests := []struct {
		name     string
		ep       config.ClusterEndpoint
		expected string
	}{
		{"URL", config.ClusterEndpoint{URL: "http://host:8080"}, "http://host:8080"},
		{"Target", config.ClusterEndpoint{Target: "dns:///host:9090"}, "dns:///host:9090"},
		{"Addr", config.ClusterEndpoint{Addr: "host:20880"}, "host:20880"},
		{"URL priority", config.ClusterEndpoint{URL: "http://url", Target: "target", Addr: "addr"}, "http://url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EndpointAddress(tt.ep)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestConfigStore_AtomicSwap(t *testing.T) {
	store := NewConfigStore()

	// Initially nil
	if store.Load() != nil {
		t.Error("expected nil initially")
	}

	// Store and load
	cfg1 := &CompiledConfig{Version: 1}
	store.Store(cfg1)
	loaded := store.Load()
	if loaded == nil || loaded.Version != 1 {
		t.Error("expected version 1")
	}

	// Atomic swap
	cfg2 := &CompiledConfig{Version: 2}
	store.Store(cfg2)
	loaded = store.Load()
	if loaded == nil || loaded.Version != 2 {
		t.Error("expected version 2 after swap")
	}
}

func TestCompileAndStore(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "test", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "test-route",
				Match:    config.RouteMatch{PathPrefix: "/"},
				Upstream: config.RouteUpstream{Cluster: "test"},
			},
		},
	}

	store := NewConfigStore()
	compiled, err := CompileAndStore(cfg, store)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	if compiled.Version == 0 {
		t.Error("expected non-zero version")
	}

	loaded := store.Load()
	if loaded == nil {
		t.Fatal("expected non-nil loaded config")
	}
	if loaded.Version != compiled.Version {
		t.Errorf("expected version %d, got %d", compiled.Version, loaded.Version)
	}
}

func TestCompile_InvalidFilterType(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "test", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:  "test-route",
				Match: config.RouteMatch{PathPrefix: "/"},
				Filters: []config.RouteFilter{
					{Type: "unknown_filter"},
				},
				Upstream: config.RouteUpstream{Cluster: "test"},
			},
		},
	}

	_, err := Compile(cfg, 1)
	if err == nil {
		t.Fatal("expected error for unknown filter type")
	}
	if !strings.Contains(err.Error(), "unknown filter type") {
		t.Errorf("expected 'unknown filter type' in error, got %s", err.Error())
	}
}

func TestCompile_DefaultValues(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{
				Name:      "test",
				Endpoints: []config.ClusterEndpoint{{URL: "http://test:8080"}},
			},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "test-route",
				Match:    config.RouteMatch{PathPrefix: "/"},
				Upstream: config.RouteUpstream{Cluster: "test"},
			},
		},
	}

	compiled, err := Compile(cfg, 1)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	cluster := compiled.Clusters["test"]
	if cluster.Type != "http" {
		t.Errorf("expected default type 'http', got %s", cluster.Type)
	}
	if cluster.LB != "round_robin" {
		t.Errorf("expected default lb 'round_robin', got %s", cluster.LB)
	}
}

func TestCompiledMatch_AllMethods(t *testing.T) {
	// When Methods is nil, all methods should match
	match := &CompiledMatch{
		Path: "/api/test",
	}

	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		req := httptest.NewRequest(method, "/api/test", nil)
		if !match.Matches(req) {
			t.Errorf("expected %s to match when methods is nil", method)
		}
	}
}

func TestGateway_NoConfig(t *testing.T) {
	store := NewConfigStore()
	gateway := NewGateway(store)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	gateway.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestGateway_NoMatch(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "test", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "api-route",
				Match:    config.RouteMatch{PathPrefix: "/api"},
				Upstream: config.RouteUpstream{Cluster: "test"},
			},
		},
	}

	store := NewConfigStore()
	if _, err := CompileAndStore(cfg, store); err != nil {
		t.Fatalf("compile error: %v", err)
	}

	gateway := NewGateway(store)
	req := httptest.NewRequest("GET", "/web/page", nil)
	w := httptest.NewRecorder()

	gateway.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGateway_ClusterNotFound(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{Name: "existing", Type: "http", Endpoints: []config.ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []config.RouteV2{
			{
				Name:     "bad-route",
				Match:    config.RouteMatch{PathPrefix: "/"},
				Upstream: config.RouteUpstream{Cluster: "nonexistent"},
			},
		},
	}

	store := NewConfigStore()
	if _, err := CompileAndStore(cfg, store); err != nil {
		t.Fatalf("compile error: %v", err)
	}

	gateway := NewGateway(store)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	gateway.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestCompile_GraphQLRoute(t *testing.T) {
	cfg := &config.Config{
		Clusters: []config.Cluster{
			{
				Name: "graphql-svc",
				Type: "graphql",
				Endpoints: []config.ClusterEndpoint{
					{URL: "http://graphql-svc:8080"},
				},
				GraphQL: &config.ClusterGraphQL{
					MaxBodyBytes: 1048576,
				},
			},
		},
		RoutesV2: []config.RouteV2{
			{
				Name: "graphql_proxy",
				Match: config.RouteMatch{
					Methods:    []string{"POST", "GET"},
					PathPrefix: "/graphql",
				},
				Upstream: config.RouteUpstream{
					Cluster: "graphql-svc",
					GraphQL: &config.RouteUpstreamGraphQL{
						Endpoint: "/graphql",
					},
				},
			},
		},
	}

	compiled, err := Compile(cfg, 4)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Test route matching
	req := httptest.NewRequest("POST", "/graphql", nil)
	route, matched := compiled.Router.Match(req)
	if !matched {
		t.Fatal("expected match")
	}
	if route.Name != "graphql_proxy" {
		t.Errorf("expected route graphql_proxy, got %s", route.Name)
	}
	if route.Upstream.ClusterName != "graphql-svc" {
		t.Errorf("expected cluster graphql-svc, got %s", route.Upstream.ClusterName)
	}
	if route.Upstream.GraphQL == nil {
		t.Fatal("expected GraphQL upstream config")
	}
	if route.Upstream.GraphQL.Endpoint != "/graphql" {
		t.Errorf("expected endpoint /graphql, got %s", route.Upstream.GraphQL.Endpoint)
	}

	// Verify cluster config
	cluster := compiled.Clusters["graphql-svc"]
	if cluster == nil {
		t.Fatal("cluster graphql-svc not found")
	}
	if cluster.Type != "graphql" {
		t.Errorf("expected type graphql, got %s", cluster.Type)
	}
	if cluster.GraphQL == nil {
		t.Fatal("expected GraphQL cluster config")
	}
	if cluster.GraphQL.MaxBodyBytes != 1048576 {
		t.Errorf("expected max_body_bytes 1048576, got %d", cluster.GraphQL.MaxBodyBytes)
	}

	// GET should also match
	req = httptest.NewRequest("GET", "/graphql", nil)
	_, matched = compiled.Router.Match(req)
	if !matched {
		t.Error("expected GET to match graphql route")
	}
}
