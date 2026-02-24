package plugin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubPlugin is a test helper that records calls and can be configured.
type stubPlugin struct {
	name    string
	order   int
	called  bool
	callIdx int
	err     error
	skipNext bool
}

func (s *stubPlugin) Name() string { return s.name }
func (s *stubPlugin) Order() int   { return s.order }
func (s *stubPlugin) Execute(ctx *GatewayContext, next func()) error {
	s.called = true
	if s.err != nil {
		return s.err
	}
	if !s.skipNext {
		next()
	}
	return nil
}

func TestChain_OrderedExecution(t *testing.T) {
	var order []string
	makePlugin := func(name string, ord int) Plugin {
		return &orderRecorder{name: name, order: ord, log: &order}
	}

	chain := NewChain(
		makePlugin("c", 30),
		makePlugin("a", 10),
		makePlugin("b", 20),
	)

	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/test", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 plugins executed, got %d", len(order))
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("unexpected order: %v, want [a b c]", order)
	}
}

func TestChain_ErrorStopsChain(t *testing.T) {
	errPlugin := &stubPlugin{name: "fail", order: 10, err: errors.New("fail")}
	neverCalled := &stubPlugin{name: "never", order: 20}

	chain := NewChain(errPlugin, neverCalled)
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	err := chain.Execute(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errPlugin.called {
		t.Error("error plugin should have been called")
	}
	if neverCalled.called {
		t.Error("next plugin should not have been called after error")
	}
}

func TestChain_SkipNextStopsChain(t *testing.T) {
	skip := &stubPlugin{name: "skip", order: 10, skipNext: true}
	neverCalled := &stubPlugin{name: "never", order: 20}

	chain := NewChain(skip, neverCalled)
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skip.called {
		t.Error("skip plugin should have been called")
	}
	if neverCalled.called {
		t.Error("next plugin should not have been called when next() was skipped")
	}
}

func TestChain_EmptyChain(t *testing.T) {
	chain := NewChain()
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}
	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error on empty chain: %v", err)
	}
}

func TestChain_AttributePropagation(t *testing.T) {
	setter := &attrSetter{name: "setter", order: 10, key: "user", value: "alice"}
	reader := &attrReader{name: "reader", order: 20, key: "user"}

	chain := NewChain(reader, setter) // reversed — chain sorts by Order
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reader.found != "alice" {
		t.Errorf("expected attribute 'alice', got %q", reader.found)
	}
}

func TestChain_Handler(t *testing.T) {
	p := &stubPlugin{name: "ok", order: 10}
	chain := NewChain(p)
	handler := chain.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if !p.called {
		t.Error("plugin should have been called via Handler()")
	}
}

func TestChain_HandlerErrorReturns500(t *testing.T) {
	p := &stubPlugin{name: "fail", order: 10, err: errors.New("boom"), skipNext: true}
	chain := NewChain(p)
	handler := chain.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestGatewayContext_RuleNil(t *testing.T) {
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}
	if ctx.Rule != nil {
		t.Error("Rule should be nil by default")
	}
}

func TestRuleData_Fields(t *testing.T) {
	rule := &RuleData{
		Name:       "test-route",
		Upstream:   "127.0.0.1:9001",
		PathPrefix: "/api",
		Host:       "example.com",
	}
	if rule.Name != "test-route" {
		t.Errorf("unexpected Name: %s", rule.Name)
	}
	if rule.Upstream != "127.0.0.1:9001" {
		t.Errorf("unexpected Upstream: %s", rule.Upstream)
	}
	if rule.PathPrefix != "/api" {
		t.Errorf("unexpected PathPrefix: %s", rule.PathPrefix)
	}
	if rule.Host != "example.com" {
		t.Errorf("unexpected Host: %s", rule.Host)
	}
}

// --- helper plugins ---

type orderRecorder struct {
	name  string
	order int
	log   *[]string
}

func (o *orderRecorder) Name() string { return o.name }
func (o *orderRecorder) Order() int   { return o.order }
func (o *orderRecorder) Execute(ctx *GatewayContext, next func()) error {
	*o.log = append(*o.log, o.name)
	next()
	return nil
}

type attrSetter struct {
	name  string
	order int
	key   string
	value interface{}
}

func (a *attrSetter) Name() string { return a.name }
func (a *attrSetter) Order() int   { return a.order }
func (a *attrSetter) Execute(ctx *GatewayContext, next func()) error {
	ctx.Attributes[a.key] = a.value
	next()
	return nil
}

type attrReader struct {
	name  string
	order int
	key   string
	found interface{}
}

func (a *attrReader) Name() string { return a.name }
func (a *attrReader) Order() int   { return a.order }
func (a *attrReader) Execute(ctx *GatewayContext, next func()) error {
	a.found = ctx.Attributes[a.key]
	next()
	return nil
}
