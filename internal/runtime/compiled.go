package runtime

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/oriys/nexus/internal/config"
)

// CompiledConfig is the pre-compiled, read-only configuration used at request time.
// It is built from the DSL config and atomically swapped for hot updates.
type CompiledConfig struct {
	Listeners []config.Listener
	Router    *RouterIndex
	Clusters  map[string]*CompiledCluster
	Filters   *FilterRegistry
	Version   uint64
}

// CompiledCluster holds a pre-compiled cluster with resolved endpoints.
type CompiledCluster struct {
	Name      string
	Type      string // "http", "grpc", "dubbo"
	Endpoints []config.ClusterEndpoint
	LB        string
	Keepalive *config.KeepaliveConfig
	GRPC      *config.ClusterGRPC
	Dubbo     *config.ClusterDubbo
	counter   atomic.Uint64
}

// NextEndpoint returns the next endpoint using round-robin load balancing.
func (c *CompiledCluster) NextEndpoint() (config.ClusterEndpoint, bool) {
	if len(c.Endpoints) == 0 {
		return config.ClusterEndpoint{}, false
	}
	idx := c.counter.Add(1) - 1
	return c.Endpoints[idx%uint64(len(c.Endpoints))], true
}

// EndpointAddress returns the effective address of an endpoint.
func EndpointAddress(ep config.ClusterEndpoint) string {
	if ep.URL != "" {
		return ep.URL
	}
	if ep.Target != "" {
		return ep.Target
	}
	return ep.Addr
}

// CompiledRoute holds a pre-compiled route with resolved filters and upstream.
type CompiledRoute struct {
	Name      string
	Match     CompiledMatch
	Filters   []Filter
	Upstream  RouteUpstreamConfig
	TimeoutMs int
}

// RouteUpstreamConfig holds the upstream configuration for a compiled route.
type RouteUpstreamConfig struct {
	ClusterName string
	GRPC        *config.RouteUpstreamGRPC
	Dubbo       *config.RouteUpstreamDubbo
}

// CompiledMatch holds pre-compiled match criteria for fast evaluation.
type CompiledMatch struct {
	Methods    map[string]struct{} // nil means match all
	Path       string              // exact path match (empty = not used)
	PathPrefix string              // prefix match (empty = not used)
	Headers    []CompiledHeaderMatch
}

// CompiledHeaderMatch is a pre-compiled header matcher.
type CompiledHeaderMatch struct {
	Name     string
	Exact    string
	Contains string
}

// Matches returns true if the request matches this compiled match.
func (m *CompiledMatch) Matches(r *http.Request) bool {
	// Check method
	if m.Methods != nil {
		if _, ok := m.Methods[r.Method]; !ok {
			return false
		}
	}

	path := r.URL.Path

	// Check exact path
	if m.Path != "" {
		if path != m.Path {
			return false
		}
	}

	// Check path prefix
	if m.PathPrefix != "" {
		if !strings.HasPrefix(path, m.PathPrefix) {
			return false
		}
	}

	// Check headers
	for _, h := range m.Headers {
		val := r.Header.Get(h.Name)
		if h.Exact != "" && val != h.Exact {
			return false
		}
		if h.Contains != "" && !strings.Contains(val, h.Contains) {
			return false
		}
	}

	return true
}

// RouterIndex provides O(1)/O(logN) route matching.
type RouterIndex struct {
	// exactRoutes maps "METHOD|path" → *CompiledRoute for O(1) exact lookups.
	exactRoutes map[string]*CompiledRoute
	// prefixRoutes is sorted by prefix length (longest first) for longest-prefix matching.
	prefixRoutes []*prefixRouteEntry
}

type prefixRouteEntry struct {
	prefix string
	route  *CompiledRoute
}

// Match finds the best matching route for the request.
func (ri *RouterIndex) Match(r *http.Request) (*CompiledRoute, bool) {
	if ri == nil {
		return nil, false
	}

	path := r.URL.Path
	method := r.Method

	// Try exact match first: "METHOD|path"
	key := method + "|" + path
	if route, ok := ri.exactRoutes[key]; ok {
		if route.Match.Matches(r) {
			return route, true
		}
	}
	// Try without method for wildcard method routes
	key = "|" + path
	if route, ok := ri.exactRoutes[key]; ok {
		if route.Match.Matches(r) {
			return route, true
		}
	}

	// Try prefix match (longest prefix wins)
	for _, pe := range ri.prefixRoutes {
		if strings.HasPrefix(path, pe.prefix) {
			if pe.route.Match.Matches(r) {
				return pe.route, true
			}
		}
	}

	return nil, false
}

// ConfigStore provides atomic access to the current CompiledConfig.
type ConfigStore struct {
	current atomic.Value // stores *CompiledConfig
}

// NewConfigStore creates a new ConfigStore.
func NewConfigStore() *ConfigStore {
	return &ConfigStore{}
}

// Store atomically stores a new CompiledConfig.
func (s *ConfigStore) Store(cfg *CompiledConfig) {
	s.current.Store(cfg)
}

// Load atomically loads the current CompiledConfig.
func (s *ConfigStore) Load() *CompiledConfig {
	v := s.current.Load()
	if v == nil {
		return nil
	}
	return v.(*CompiledConfig)
}
