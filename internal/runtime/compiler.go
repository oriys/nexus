package runtime

import (
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/oriys/nexus/internal/config"
)

// Compile compiles a Config into a CompiledConfig for fast request-time lookups.
func Compile(cfg *config.Config, version uint64) (*CompiledConfig, error) {
	fr := NewFilterRegistry()

	// Compile clusters
	clusters := make(map[string]*CompiledCluster, len(cfg.Clusters))
	for _, c := range cfg.Clusters {
		cc := &CompiledCluster{
			Name:      c.Name,
			Type:      c.Type,
			Endpoints: c.Endpoints,
			LB:        c.LB,
			Keepalive: c.Keepalive,
			GRPC:      c.GRPC,
			Dubbo:     c.Dubbo,
		}
		if cc.LB == "" {
			cc.LB = "round_robin"
		}
		if cc.Type == "" {
			cc.Type = "http"
		}
		clusters[c.Name] = cc
	}

	// Compile routes
	exactRoutes := make(map[string]*CompiledRoute)
	var prefixRoutes []*prefixRouteEntry

	for _, rv2 := range cfg.RoutesV2 {
		// Compile match
		cm := CompiledMatch{
			Path:       rv2.Match.Path,
			PathPrefix: rv2.Match.PathPrefix,
		}

		if len(rv2.Match.Methods) > 0 {
			cm.Methods = make(map[string]struct{}, len(rv2.Match.Methods))
			for _, m := range rv2.Match.Methods {
				cm.Methods[m] = struct{}{}
			}
		}

		for _, h := range rv2.Match.Headers {
			cm.Headers = append(cm.Headers, CompiledHeaderMatch{
				Name:     h.Name,
				Exact:    h.Exact,
				Contains: h.Contains,
			})
		}

		// Compile filters
		var filters []Filter
		for _, rf := range rv2.Filters {
			f, err := fr.Compile(rf)
			if err != nil {
				return nil, fmt.Errorf("route %q filter %q: %w", rv2.Name, rf.Type, err)
			}
			filters = append(filters, f)
		}

		cr := &CompiledRoute{
			Name:    rv2.Name,
			Match:   cm,
			Filters: filters,
			Upstream: RouteUpstreamConfig{
				ClusterName: rv2.Upstream.Cluster,
				GRPC:        rv2.Upstream.GRPC,
				Dubbo:       rv2.Upstream.Dubbo,
			},
			TimeoutMs: rv2.Upstream.TimeoutMs,
		}

		// Index the route
		if cm.Path != "" {
			// Exact path routes go into the exact map
			if cm.Methods != nil {
				for m := range cm.Methods {
					key := m + "|" + cm.Path
					exactRoutes[key] = cr
				}
			} else {
				key := "|" + cm.Path
				exactRoutes[key] = cr
			}
		}

		if cm.PathPrefix != "" {
			// Prefix routes go into the prefix list
			prefixRoutes = append(prefixRoutes, &prefixRouteEntry{
				prefix: cm.PathPrefix,
				route:  cr,
			})
		}

		// If neither path nor prefix is set, this is a catch-all (treated as "/" prefix)
		if cm.Path == "" && cm.PathPrefix == "" {
			prefixRoutes = append(prefixRoutes, &prefixRouteEntry{
				prefix: "/",
				route:  cr,
			})
		}
	}

	// Sort prefix routes by length descending (longest match first).
	// Use lexicographic ordering as tiebreaker for deterministic matching.
	sort.Slice(prefixRoutes, func(i, j int) bool {
		if len(prefixRoutes[i].prefix) != len(prefixRoutes[j].prefix) {
			return len(prefixRoutes[i].prefix) > len(prefixRoutes[j].prefix)
		}
		return prefixRoutes[i].prefix < prefixRoutes[j].prefix
	})

	router := &RouterIndex{
		exactRoutes:  exactRoutes,
		prefixRoutes: prefixRoutes,
	}

	return &CompiledConfig{
		Listeners: cfg.Listeners,
		Router:    router,
		Clusters:  clusters,
		Filters:   fr,
		Version:   version,
	}, nil
}

// versionCounter is used to generate unique version numbers for compiled configs.
var versionCounter atomic.Uint64

// CompileAndStore compiles the config and stores it atomically.
func CompileAndStore(cfg *config.Config, store *ConfigStore) (*CompiledConfig, error) {
	version := versionCounter.Add(1)
	compiled, err := Compile(cfg, version)
	if err != nil {
		return nil, err
	}
	store.Store(compiled)
	return compiled, nil
}
