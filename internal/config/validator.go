package config

import (
	"errors"
	"fmt"
)

// Validate checks the configuration for correctness.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	// server.listen is required unless listeners are defined
	if cfg.Server.Listen == "" && len(cfg.Listeners) == 0 {
		return errors.New("server.listen is required (or define listeners)")
	}

	upstreamNames := make(map[string]bool)
	for i, u := range cfg.Upstreams {
		if u.Name == "" {
			return fmt.Errorf("upstreams[%d].name is required", i)
		}
		if upstreamNames[u.Name] {
			return fmt.Errorf("duplicate upstream name: %s", u.Name)
		}
		upstreamNames[u.Name] = true

		if len(u.Targets) == 0 {
			return fmt.Errorf("upstream %q must have at least one target", u.Name)
		}
		for j, t := range u.Targets {
			if t.Address == "" {
				return fmt.Errorf("upstream %q target[%d].address is required", u.Name, j)
			}
		}
	}

	for i, r := range cfg.Routes {
		if r.Name == "" {
			return fmt.Errorf("routes[%d].name is required", i)
		}
		if r.Upstream == "" {
			return fmt.Errorf("route %q upstream is required", r.Name)
		}
		if !upstreamNames[r.Upstream] {
			return fmt.Errorf("route %q references unknown upstream %q", r.Name, r.Upstream)
		}
		if len(r.Paths) == 0 {
			return fmt.Errorf("route %q must have at least one path rule", r.Name)
		}
		for j, p := range r.Paths {
			if p.Path == "" {
				return fmt.Errorf("route %q paths[%d].path is required", r.Name, j)
			}
			if p.Type != "exact" && p.Type != "prefix" {
				return fmt.Errorf("route %q paths[%d].type must be 'exact' or 'prefix', got %q", r.Name, j, p.Type)
			}
		}
		if err := validateRewrite(r.Name, r.Rewrite); err != nil {
			return err
		}
	}

	// Validate new DSL structures (listeners, clusters, routes_v2)
	if err := validateListeners(cfg.Listeners); err != nil {
		return err
	}

	clusterNames := make(map[string]bool)
	if err := validateClusters(cfg.Clusters, clusterNames); err != nil {
		return err
	}

	if err := validateRoutesV2(cfg.RoutesV2, clusterNames); err != nil {
		return err
	}

	return nil
}

// validateListeners validates listener configurations.
func validateListeners(listeners []Listener) error {
	names := make(map[string]bool)
	for i, l := range listeners {
		if l.Name == "" {
			return fmt.Errorf("listeners[%d].name is required", i)
		}
		if names[l.Name] {
			return fmt.Errorf("duplicate listener name: %s", l.Name)
		}
		names[l.Name] = true
		if l.Addr == "" {
			return fmt.Errorf("listener %q addr is required", l.Name)
		}
	}
	return nil
}

// validateClusters validates cluster configurations.
func validateClusters(clusters []Cluster, clusterNames map[string]bool) error {
	for i, c := range clusters {
		if c.Name == "" {
			return fmt.Errorf("clusters[%d].name is required", i)
		}
		if clusterNames[c.Name] {
			return fmt.Errorf("duplicate cluster name: %s", c.Name)
		}
		clusterNames[c.Name] = true

		switch c.Type {
		case "", "http", "grpc", "dubbo":
			// valid
		default:
			return fmt.Errorf("cluster %q: unsupported type %q, must be 'http', 'grpc', or 'dubbo'", c.Name, c.Type)
		}

		if len(c.Endpoints) == 0 {
			return fmt.Errorf("cluster %q must have at least one endpoint", c.Name)
		}
		for j, ep := range c.Endpoints {
			if ep.URL == "" && ep.Target == "" && ep.Addr == "" {
				return fmt.Errorf("cluster %q endpoint[%d]: url, target, or addr is required", c.Name, j)
			}
		}

		if c.Type == "grpc" && c.GRPC == nil {
			// grpc cluster config is optional, just use defaults
		}
		if c.Type == "dubbo" && c.Dubbo == nil {
			// dubbo cluster config is optional, just use defaults
		}
	}
	return nil
}

// validateRoutesV2 validates V2 route configurations.
func validateRoutesV2(routes []RouteV2, clusterNames map[string]bool) error {
	for i, r := range routes {
		if r.Name == "" {
			return fmt.Errorf("routes_v2[%d].name is required", i)
		}

		if r.Match.Path == "" && r.Match.PathPrefix == "" {
			return fmt.Errorf("route_v2 %q: match.path or match.path_prefix is required", r.Name)
		}

		if r.Upstream.Cluster == "" {
			return fmt.Errorf("route_v2 %q: upstream.cluster is required", r.Name)
		}

		if len(clusterNames) > 0 && !clusterNames[r.Upstream.Cluster] {
			return fmt.Errorf("route_v2 %q references unknown cluster %q", r.Name, r.Upstream.Cluster)
		}

		// Validate filters
		for j, f := range r.Filters {
			if f.Type == "" {
				return fmt.Errorf("route_v2 %q filters[%d].type is required", r.Name, j)
			}
			switch f.Type {
			case "strip_prefix":
				if f.Args == nil || f.Args["prefix"] == "" {
					return fmt.Errorf("route_v2 %q filters[%d] (strip_prefix): 'prefix' argument is required", r.Name, j)
				}
			case "header_set":
				if f.Args == nil || f.Args["key"] == "" {
					return fmt.Errorf("route_v2 %q filters[%d] (header_set): 'key' argument is required", r.Name, j)
				}
			}
		}

		// Validate gRPC upstream config
		if r.Upstream.GRPC != nil {
			if r.Upstream.GRPC.Service == "" {
				return fmt.Errorf("route_v2 %q: upstream.grpc.service is required", r.Name)
			}
			if r.Upstream.GRPC.Method == "" {
				return fmt.Errorf("route_v2 %q: upstream.grpc.method is required", r.Name)
			}
		}

		// Validate Dubbo upstream config
		if r.Upstream.Dubbo != nil {
			if r.Upstream.Dubbo.Interface == "" {
				return fmt.Errorf("route_v2 %q: upstream.dubbo.interface is required", r.Name)
			}
			if r.Upstream.Dubbo.Method == "" {
				return fmt.Errorf("route_v2 %q: upstream.dubbo.method is required", r.Name)
			}
		}
	}
	return nil
}

// validateRewrite validates the rewrite rules for a route.
func validateRewrite(routeName string, rw *RewriteRule) error {
	if rw == nil {
		return nil
	}

	switch rw.Protocol {
	case "", "http":
		// valid, http is default
	case "grpc":
		if rw.GRPC == nil {
			return fmt.Errorf("route %q: grpc rewrite requires grpc configuration", routeName)
		}
		if rw.GRPC.Service == "" {
			return fmt.Errorf("route %q: grpc.service is required", routeName)
		}
		if rw.GRPC.Method == "" {
			return fmt.Errorf("route %q: grpc.method is required", routeName)
		}
	case "dubbo":
		if rw.Dubbo == nil {
			return fmt.Errorf("route %q: dubbo rewrite requires dubbo configuration", routeName)
		}
		if rw.Dubbo.Service == "" {
			return fmt.Errorf("route %q: dubbo.service is required", routeName)
		}
		if rw.Dubbo.Method == "" {
			return fmt.Errorf("route %q: dubbo.method is required", routeName)
		}
	default:
		return fmt.Errorf("route %q: unsupported rewrite protocol %q, must be 'http', 'grpc', or 'dubbo'", routeName, rw.Protocol)
	}

	return nil
}
