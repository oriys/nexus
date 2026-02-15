package config

import (
	"testing"
)

func TestValidateValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{
				Name:    "backend",
				Targets: []Target{{Address: "127.0.0.1:9001"}},
			},
		},
		Routes: []Route{
			{
				Name:     "api",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateNilConfig(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Error("expected error for nil config")
	}
}

func TestValidateMissingListen(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Listen: ""}}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing listen")
	}
}

func TestValidateDuplicateUpstream(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "a", Targets: []Target{{Address: "1.2.3.4:80"}}},
			{Name: "a", Targets: []Target{{Address: "5.6.7.8:80"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for duplicate upstream")
	}
}

func TestValidateUpstreamNoTargets(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "empty", Targets: []Target{}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for upstream with no targets")
	}
}

func TestValidateRouteUnknownUpstream(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "bad", Upstream: "nonexistent", Paths: []PathRule{{Path: "/", Type: "prefix"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for unknown upstream reference")
	}
}

func TestValidateRouteInvalidPathType(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "bad", Upstream: "backend", Paths: []PathRule{{Path: "/", Type: "regex"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid path type")
	}
}

func TestValidateRouteMissingName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "", Upstream: "backend", Paths: []PathRule{{Path: "/", Type: "prefix"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing route name")
	}
}

func TestValidateUpstreamMissingName(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing upstream name")
	}
}

func TestValidateTargetMissingAddress(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: ""}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing target address")
	}
}

func TestValidateRouteMissingUpstream(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "route1", Upstream: "", Paths: []PathRule{{Path: "/", Type: "prefix"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing route upstream")
	}
}

func TestValidateRouteNoPaths(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "route1", Upstream: "backend", Paths: []PathRule{}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for route with no paths")
	}
}

func TestValidatePathMissingPath(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{Name: "route1", Upstream: "backend", Paths: []PathRule{{Path: "", Type: "prefix"}}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing path")
	}
}

func TestValidateRewrite_ValidHTTP(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/api", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "http",
					PathRewrite: &PathRewrite{
						Prefix: "/internal",
					},
					Headers: &HeaderRewrite{
						Add: map[string]string{"X-Custom": "value"},
					},
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateRewrite_ValidGRPC(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "grpc-backend", Targets: []Target{{Address: "127.0.0.1:50051"}}},
		},
		Routes: []Route{
			{
				Name:     "grpc-route",
				Upstream: "grpc-backend",
				Paths:    []PathRule{{Path: "/api/hello", Type: "exact"}},
				Rewrite: &RewriteRule{
					Protocol: "grpc",
					GRPC: &GRPCRewrite{
						Service: "helloworld.Greeter",
						Method:  "SayHello",
					},
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateRewrite_ValidDubbo(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "dubbo-backend", Targets: []Target{{Address: "127.0.0.1:20880"}}},
		},
		Routes: []Route{
			{
				Name:     "dubbo-route",
				Upstream: "dubbo-backend",
				Paths:    []PathRule{{Path: "/api/user", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "dubbo",
					Dubbo: &DubboRewrite{
						Service: "com.example.UserService",
						Method:  "getUser",
						Group:   "default",
						Version: "1.0.0",
					},
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateRewrite_InvalidProtocol(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "websocket",
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for unsupported protocol")
	}
}

func TestValidateRewrite_GRPCMissingConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "grpc",
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for grpc without grpc config")
	}
}

func TestValidateRewrite_GRPCMissingService(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "grpc",
					GRPC:     &GRPCRewrite{Method: "SayHello"},
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for grpc missing service")
	}
}

func TestValidateRewrite_GRPCMissingMethod(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "grpc",
					GRPC:     &GRPCRewrite{Service: "helloworld.Greeter"},
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for grpc missing method")
	}
}

func TestValidateRewrite_DubboMissingConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "dubbo",
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for dubbo without dubbo config")
	}
}

func TestValidateRewrite_DubboMissingService(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "dubbo",
					Dubbo:    &DubboRewrite{Method: "getUser"},
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for dubbo missing service")
	}
}

func TestValidateRewrite_DubboMissingMethod(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					Protocol: "dubbo",
					Dubbo:    &DubboRewrite{Service: "com.example.UserService"},
				},
			},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for dubbo missing method")
	}
}

func TestValidateRewrite_NilRewrite(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error for nil rewrite, got %v", err)
	}
}

func TestValidateRewrite_EmptyProtocol(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8080"},
		Upstreams: []Upstream{
			{Name: "backend", Targets: []Target{{Address: "127.0.0.1:80"}}},
		},
		Routes: []Route{
			{
				Name:     "route1",
				Upstream: "backend",
				Paths:    []PathRule{{Path: "/", Type: "prefix"}},
				Rewrite: &RewriteRule{
					PathRewrite: &PathRewrite{Prefix: "/v2"},
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected no error for empty protocol (defaults to http), got %v", err)
	}
}
