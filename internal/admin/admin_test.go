package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/oriys/nexus/internal/config"
	"github.com/oriys/nexus/internal/proxy"
)

const testConfig = `server:
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

func setupAdmin(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nexus.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}
	cl := config.NewLoader(cfgPath)
	cfg, err := cl.Load()
	if err != nil {
		t.Fatal(err)
	}
	vm := config.NewVersionManager(10)
	vm.Save(cfg, []byte(testConfig))
	r := proxy.NewRouter()
	r.Reload(cfg.Routes)
	um := proxy.NewUpstreamManager()
	um.Reload(cfg.Upstreams)
	return New(cl, vm, r, um)
}

func TestGetConfig(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["Routes"]; !ok {
		t.Fatal("expected routes in response")
	}
}

func TestGetConfig_NoConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nexus.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}
	cl := config.NewLoader(cfgPath)
	// Do not load config — Current() will return nil
	vm := config.NewVersionManager(10)
	r := proxy.NewRouter()
	um := proxy.NewUpstreamManager()
	s := New(cl, vm, r, um)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["error"] != "no configuration loaded" {
		t.Fatalf("unexpected error: %s", result["error"])
	}
}

func TestListVersions(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/versions", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result))
	}
	if _, ok := result[0]["version"]; !ok {
		t.Fatal("expected version field")
	}
	if _, ok := result[0]["hash"]; !ok {
		t.Fatal("expected hash field")
	}
	if _, ok := result[0]["timestamp"]; !ok {
		t.Fatal("expected timestamp field")
	}
}

func TestRollbackConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nexus.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}
	cl := config.NewLoader(cfgPath)
	cfg, err := cl.Load()
	if err != nil {
		t.Fatal(err)
	}
	vm := config.NewVersionManager(10)
	vm.Save(cfg, []byte(testConfig))
	// Save a second version so rollback has something to go back to
	vm.Save(cfg, []byte(testConfig))

	r := proxy.NewRouter()
	r.Reload(cfg.Routes)
	um := proxy.NewUpstreamManager()
	um.Reload(cfg.Upstreams)
	s := New(cl, vm, r, um)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/rollback", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["message"] != "configuration rolled back successfully" {
		t.Fatalf("unexpected message: %s", result["message"])
	}
}

func TestRollbackConfig_NoHistory(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nexus.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}
	cl := config.NewLoader(cfgPath)
	vm := config.NewVersionManager(10)
	r := proxy.NewRouter()
	um := proxy.NewUpstreamManager()
	s := New(cl, vm, r, um)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/rollback", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["error"] == "" {
		t.Fatal("expected error message")
	}
}

func TestListRoutes(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 route, got %d", len(result))
	}
	if result[0]["Name"] != "api" {
		t.Fatalf("expected route name 'api', got %v", result[0]["Name"])
	}
}

func TestListUpstreams(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(result))
	}
	if result[0]["Name"] != "backend" {
		t.Fatalf("expected upstream name 'backend', got %v", result[0]["Name"])
	}
}

func TestGetStatus(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "running" {
		t.Fatalf("expected status 'running', got %v", result["status"])
	}
	if result["config_versions"].(float64) != 1 {
		t.Fatalf("expected 1 config version, got %v", result["config_versions"])
	}
}
