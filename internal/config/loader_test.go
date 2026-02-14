package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
server:
  listen: ":8080"
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 30s

upstreams:
  - name: backend
    targets:
      - address: "127.0.0.1:9001"
        weight: 1

routes:
  - name: api
    host: ""
    paths:
      - path: /
        type: prefix
    upstream: backend

logging:
  level: info
  format: json
`
	path := writeTemp(t, content)
	loader := NewLoader(path)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Errorf("expected listen :8080, got %s", cfg.Server.Listen)
	}
	if len(cfg.Upstreams) != 1 {
		t.Errorf("expected 1 upstream, got %d", len(cfg.Upstreams))
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(cfg.Routes))
	}
	if cfg.Upstreams[0].Name != "backend" {
		t.Errorf("expected upstream name backend, got %s", cfg.Upstreams[0].Name)
	}

	// Test Current()
	cur := loader.Current()
	if cur == nil {
		t.Fatal("Current() should return loaded config")
	}
	if cur.Server.Listen != cfg.Server.Listen {
		t.Error("Current() should match loaded config")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTemp(t, "{{invalid yaml")
	loader := NewLoader(path)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	loader := NewLoader("/nonexistent/path.yaml")
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	content := `
server:
  listen: ""
upstreams: []
routes: []
`
	path := writeTemp(t, content)
	loader := NewLoader(path)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCurrentReturnsNilBeforeLoad(t *testing.T) {
	loader := NewLoader("nonexistent.yaml")
	if loader.Current() != nil {
		t.Error("Current() should return nil before Load()")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
