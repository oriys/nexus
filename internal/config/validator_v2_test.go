package config

import (
	"strings"
	"testing"
)

func TestValidateV2_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Listeners: []Listener{
			{Name: "public", Addr: ":8080"},
		},
		Clusters: []Cluster{
			{
				Name:      "user-http",
				Type:      "http",
				Endpoints: []ClusterEndpoint{{URL: "http://user-svc:8080"}},
				LB:        "round_robin",
			},
		},
		RoutesV2: []RouteV2{
			{
				Name: "http_passthrough",
				Match: RouteMatch{
					Methods:    []string{"GET", "POST"},
					PathPrefix: "/api/v1/http/",
				},
				Filters: []RouteFilter{
					{Type: "strip_prefix", Args: map[string]string{"prefix": "/api/v1/http"}},
				},
				Upstream: RouteUpstream{Cluster: "user-http", TimeoutMs: 30000},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateV2_ListenerMissingName(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Listen: ":8080"},
		Listeners: []Listener{{Name: "", Addr: ":8080"}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing listener name")
	}
	if !strings.Contains(err.Error(), "listeners[0].name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateV2_ListenerMissingAddr(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Listen: ":8080"},
		Listeners: []Listener{{Name: "public", Addr: ""}},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing listener addr")
	}
	if !strings.Contains(err.Error(), "addr is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateV2_DuplicateListenerName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Listeners: []Listener{
			{Name: "public", Addr: ":8080"},
			{Name: "public", Addr: ":8443"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate listener name")
	}
	if !strings.Contains(err.Error(), "duplicate listener name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateV2_ClusterMissingName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing cluster name")
	}
}

func TestValidateV2_ClusterInvalidType(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "websocket", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid cluster type")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateV2_ClusterNoEndpoints(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for cluster with no endpoints")
	}
}

func TestValidateV2_ClusterEndpointEmpty(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{}}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if !strings.Contains(err.Error(), "url, target, or addr is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateV2_DuplicateClusterName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://a:8080"}}},
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://b:8080"}}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate cluster name")
	}
}

func TestValidateV2_RouteV2MissingName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:     "",
				Match:    RouteMatch{PathPrefix: "/"},
				Upstream: RouteUpstream{Cluster: "test"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing route name")
	}
}

func TestValidateV2_RouteV2MissingPath(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:     "test",
				Match:    RouteMatch{},
				Upstream: RouteUpstream{Cluster: "test"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing path and path_prefix")
	}
}

func TestValidateV2_RouteV2MissingCluster(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:     "test",
				Match:    RouteMatch{PathPrefix: "/"},
				Upstream: RouteUpstream{Cluster: ""},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestValidateV2_RouteV2UnknownCluster(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:     "test",
				Match:    RouteMatch{PathPrefix: "/"},
				Upstream: RouteUpstream{Cluster: "nonexistent"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unknown cluster reference")
	}
}

func TestValidateV2_FilterMissingType(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:  "test",
				Match: RouteMatch{PathPrefix: "/"},
				Filters: []RouteFilter{
					{Type: ""},
				},
				Upstream: RouteUpstream{Cluster: "test"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing filter type")
	}
}

func TestValidateV2_StripPrefixMissingArg(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "http", Endpoints: []ClusterEndpoint{{URL: "http://test:8080"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:  "test",
				Match: RouteMatch{PathPrefix: "/"},
				Filters: []RouteFilter{
					{Type: "strip_prefix", Args: map[string]string{}},
				},
				Upstream: RouteUpstream{Cluster: "test"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for strip_prefix missing prefix arg")
	}
}

func TestValidateV2_GRPCUpstreamMissingService(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "grpc", Endpoints: []ClusterEndpoint{{Target: "dns:///test:9090"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:  "test",
				Match: RouteMatch{Path: "/api/test"},
				Upstream: RouteUpstream{
					Cluster: "test",
					GRPC:    &RouteUpstreamGRPC{Service: "", Method: "Test"},
				},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing grpc service")
	}
}

func TestValidateV2_DubboUpstreamMissingInterface(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Clusters: []Cluster{
			{Name: "test", Type: "dubbo", Endpoints: []ClusterEndpoint{{Addr: "test:20880"}}},
		},
		RoutesV2: []RouteV2{
			{
				Name:  "test",
				Match: RouteMatch{Path: "/api/test"},
				Upstream: RouteUpstream{
					Cluster: "test",
					Dubbo:   &RouteUpstreamDubbo{Interface: "", Method: "Test"},
				},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing dubbo interface")
	}
}

func TestValidateV2_ListenersCanReplaceServerListen(t *testing.T) {
	// When listeners are defined, server.listen is not required
	cfg := &Config{
		Server: ServerConfig{Listen: ""},
		Listeners: []Listener{
			{Name: "public", Addr: ":8080"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error when listeners are defined without server.listen, got %v", err)
	}
}

func TestValidateV2_FullDSLConfig(t *testing.T) {
	cfg := &Config{
		Server:  ServerConfig{Listen: ":8080"},
		Version: "v1",
		Listeners: []Listener{
			{Name: "public", Addr: ":8080", H2C: true},
		},
		Clusters: []Cluster{
			{
				Name:      "user-http",
				Type:      "http",
				Endpoints: []ClusterEndpoint{{URL: "http://user-svc:8080"}},
				LB:        "round_robin",
				Keepalive: &KeepaliveConfig{MaxIdleConns: 1024, IdleConnTimeoutMs: 60000},
			},
			{
				Name:      "user-grpc",
				Type:      "grpc",
				Endpoints: []ClusterEndpoint{{Target: "dns:///user-grpc:9090"}},
				LB:        "pick_first",
				GRPC:      &ClusterGRPC{Authority: "user-grpc", MaxRecvMsgMB: 16},
			},
			{
				Name:      "order-dubbo",
				Type:      "dubbo",
				Endpoints: []ClusterEndpoint{{Addr: "order-dubbo:20880"}},
				Dubbo: &ClusterDubbo{
					Application:   "nova-gw",
					Version:       "1.0.0",
					Serialization: "hessian2",
				},
			},
		},
		RoutesV2: []RouteV2{
			{
				Name: "http_passthrough",
				Match: RouteMatch{
					Methods:    []string{"GET", "POST", "PUT", "DELETE"},
					PathPrefix: "/api/v1/http/",
				},
				Filters: []RouteFilter{
					{Type: "strip_prefix", Args: map[string]string{"prefix": "/api/v1/http"}},
					{Type: "header_set", Args: map[string]string{"key": "x-gw", "value": "nova"}},
				},
				Upstream: RouteUpstream{Cluster: "user-http", TimeoutMs: 30000},
			},
			{
				Name: "http_to_grpc",
				Match: RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/v1/user/get",
					Headers: []HeaderMatch{{Name: "content-type", Contains: "application/json"}},
				},
				Upstream: RouteUpstream{
					Cluster: "user-grpc",
					GRPC: &RouteUpstreamGRPC{
						Service:  "user.v1.UserService",
						Method:   "GetUser",
						Request:  &TranscodeMode{Mode: "json_to_proto", Proto: "user.v1.GetUserRequest"},
						Response: &TranscodeMode{Mode: "proto_to_json"},
					},
				},
			},
			{
				Name: "http_to_dubbo",
				Match: RouteMatch{
					Methods: []string{"POST"},
					Path:    "/api/v1/order/create",
				},
				Upstream: RouteUpstream{
					Cluster: "order-dubbo",
					Dubbo: &RouteUpstreamDubbo{
						Interface:  "com.foo.order.OrderService",
						Method:     "CreateOrder",
						ParamTypes: []string{"com.foo.order.CreateOrderRequest"},
						Request:    &TranscodeMode{Mode: "json_to_hessian"},
						Response:   &TranscodeMode{Mode: "hessian_to_json"},
					},
				},
			},
		},
	}

	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error for full DSL config, got %v", err)
	}
}
