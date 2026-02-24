package plugin

import (
	"net/http/httptest"
	"testing"
)

func TestGlobalLogPlugin_Name(t *testing.T) {
	p := NewGlobalLogPlugin()
	if p.Name() != "global_log" {
		t.Errorf("expected name 'global_log', got %q", p.Name())
	}
}

func TestGlobalLogPlugin_Order(t *testing.T) {
	p := NewGlobalLogPlugin()
	if p.Order() != 0 {
		t.Errorf("expected order 0, got %d", p.Order())
	}
}

func TestGlobalLogPlugin_CallsNext(t *testing.T) {
	p := NewGlobalLogPlugin()
	nextCalled := false
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("GET", "/hello", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	err := p.Execute(ctx, func() { nextCalled = true })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nextCalled {
		t.Error("GlobalLogPlugin should call next()")
	}
}

func TestGlobalLogPlugin_InChain(t *testing.T) {
	log := NewGlobalLogPlugin()
	called := &stubPlugin{name: "downstream", order: 50}

	chain := NewChain(log, called)
	ctx := &GatewayContext{
		Request:        httptest.NewRequest("POST", "/api/test", nil),
		ResponseWriter: httptest.NewRecorder(),
		Attributes:     make(map[string]interface{}),
	}

	if err := chain.Execute(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called.called {
		t.Error("downstream plugin should have been called")
	}
}
