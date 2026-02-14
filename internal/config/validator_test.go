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
