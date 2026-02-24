package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHttpProxyPlugin_Name(t *testing.T) {
	p := NewHttpProxyPlugin()
	if p.Name() != "http_proxy" {
		t.Errorf("expected name 'http_proxy', got %q", p.Name())
	}
}

func TestHttpProxyPlugin_Order(t *testing.T) {
	p := NewHttpProxyPlugin()
	if p.Order() != 100 {
		t.Errorf("expected order 100, got %d", p.Order())
	}
}

func TestHttpProxyPlugin_NoRule(t *testing.T) {
	p := NewHttpProxyPlugin()
	rec := httptest.NewRecorder()
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: rec,
		Attributes:     make(map[string]interface{}),
		Rule:           nil,
	}

	err := p.Execute(ctx, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected %d, got %d", http.StatusBadGateway, rec.Code)
	}
}

func TestHttpProxyPlugin_EmptyUpstream(t *testing.T) {
	p := NewHttpProxyPlugin()
	rec := httptest.NewRecorder()
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: rec,
		Attributes:     make(map[string]interface{}),
		Rule:           &RuleData{Upstream: ""},
	}

	err := p.Execute(ctx, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected %d, got %d", http.StatusBadGateway, rec.Code)
	}
}

func TestHttpProxyPlugin_ProxiesToBackend(t *testing.T) {
	// Start a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "reached")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	p := NewHttpProxyPlugin()
	rec := httptest.NewRecorder()
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/api/test", nil),
		ResponseWriter: rec,
		Attributes:     make(map[string]interface{}),
		Rule:           &RuleData{Upstream: backend.URL},
	}

	err := p.Execute(ctx, func() { t.Error("next() should not be called by terminal plugin") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Header().Get("X-Backend") != "reached" {
		t.Error("backend header not propagated")
	}
	if rec.Body.String() != "hello from backend" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHttpProxyPlugin_FullChain(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied"))
	}))
	defer backend.Close()

	log := NewGlobalLogPlugin()
	proxy := NewHttpProxyPlugin()

	// A rule-setter plugin that assigns the upstream before proxy runs.
	ruleSetter := &ruleSetterPlugin{
		upstream: backend.URL,
	}

	chain := NewChain(log, ruleSetter, proxy)
	rec := httptest.NewRecorder()
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/api/test", nil),
		ResponseWriter: rec,
		Attributes:     make(map[string]interface{}),
	}

	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "proxied" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

// ruleSetterPlugin simulates route matching by setting the rule on the context.
type ruleSetterPlugin struct {
	upstream string
}

func (r *ruleSetterPlugin) Name() string { return "rule_setter" }
func (r *ruleSetterPlugin) Order() int   { return 50 }
func (r *ruleSetterPlugin) Execute(ctx *GatewayContext, next func()) error {
	ctx.Rule = &RuleData{
		Name:     "test-route",
		Upstream: r.upstream,
	}
	next()
	return nil
}
