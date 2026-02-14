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

	if cfg.Server.Listen == "" {
		return errors.New("server.listen is required")
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
	}

	return nil
}
