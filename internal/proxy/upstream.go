package proxy

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/oriys/nexus/internal/config"
)

// UpstreamManager manages upstream target groups and provides load-balanced selection.
type UpstreamManager struct {
	mu        sync.RWMutex
	upstreams map[string]*upstreamGroup
}

type upstreamGroup struct {
	name    string
	targets []config.Target
	counter atomic.Uint64
}

// NewUpstreamManager creates a new UpstreamManager.
func NewUpstreamManager() *UpstreamManager {
	return &UpstreamManager{
		upstreams: make(map[string]*upstreamGroup),
	}
}

// Reload rebuilds all upstream groups from the configuration.
func (m *UpstreamManager) Reload(upstreams []config.Upstream) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newUpstreams := make(map[string]*upstreamGroup, len(upstreams))
	for _, u := range upstreams {
		newUpstreams[u.Name] = &upstreamGroup{
			name:    u.Name,
			targets: u.Targets,
		}
	}
	m.upstreams = newUpstreams

	slog.Info("upstream groups reloaded", slog.Int("count", len(newUpstreams)))
}

// GetTarget returns the next target address for the given upstream using round-robin.
// Returns the address and whether the upstream was found.
func (m *UpstreamManager) GetTarget(upstreamName string) (string, bool) {
	m.mu.RLock()
	group, ok := m.upstreams[upstreamName]
	m.mu.RUnlock()

	if !ok || len(group.targets) == 0 {
		return "", false
	}

	// Round-robin selection using atomic counter
	idx := group.counter.Add(1) - 1
	target := group.targets[idx%uint64(len(group.targets))]
	return target.Address, true
}
