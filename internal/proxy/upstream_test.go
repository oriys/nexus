package proxy

import (
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestUpstreamManagerRoundRobin(t *testing.T) {
	mgr := NewUpstreamManager()
	mgr.Reload([]config.Upstream{
		{
			Name: "backend",
			Targets: []config.Target{
				{Address: "127.0.0.1:9001"},
				{Address: "127.0.0.1:9002"},
				{Address: "127.0.0.1:9003"},
			},
		},
	})

	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		addr, ok := mgr.GetTarget("backend")
		if !ok {
			t.Fatal("expected target")
		}
		seen[addr]++
	}

	// Each target should be selected 3 times with round-robin
	for _, addr := range []string{"127.0.0.1:9001", "127.0.0.1:9002", "127.0.0.1:9003"} {
		if seen[addr] != 3 {
			t.Errorf("expected %s to be selected 3 times, got %d", addr, seen[addr])
		}
	}
}

func TestUpstreamManagerUnknown(t *testing.T) {
	mgr := NewUpstreamManager()
	mgr.Reload([]config.Upstream{})

	_, ok := mgr.GetTarget("nonexistent")
	if ok {
		t.Error("expected no target for unknown upstream")
	}
}

func TestUpstreamManagerReload(t *testing.T) {
	mgr := NewUpstreamManager()
	mgr.Reload([]config.Upstream{
		{Name: "a", Targets: []config.Target{{Address: "1.2.3.4:80"}}},
	})

	addr, ok := mgr.GetTarget("a")
	if !ok || addr != "1.2.3.4:80" {
		t.Fatalf("expected 1.2.3.4:80, got %s", addr)
	}

	// Reload with different upstream
	mgr.Reload([]config.Upstream{
		{Name: "b", Targets: []config.Target{{Address: "5.6.7.8:80"}}},
	})

	_, ok = mgr.GetTarget("a")
	if ok {
		t.Error("upstream 'a' should not exist after reload")
	}

	addr, ok = mgr.GetTarget("b")
	if !ok || addr != "5.6.7.8:80" {
		t.Errorf("expected 5.6.7.8:80, got %s", addr)
	}
}
