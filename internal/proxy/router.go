package proxy

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/oriys/nexus/internal/config"
)

// routeEntry is an internal representation of a route for matching.
type routeEntry struct {
	route    config.Route
	upstream string
}

// Router matches incoming requests to upstream backends based on Host and Path rules.
type Router struct {
	// exact stores exact host+path → routeEntry mappings.
	exact atomic.Pointer[map[string]routeEntry]
	// prefixes stores prefix-based route entries sorted by path length (longest first).
	prefixes atomic.Pointer[[]prefixEntry]
}

type prefixEntry struct {
	host     string
	prefix   string
	entry    routeEntry
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	r := &Router{}
	empty := make(map[string]routeEntry)
	r.exact.Store(&empty)
	emptyPrefixes := make([]prefixEntry, 0)
	r.prefixes.Store(&emptyPrefixes)
	return r
}

// Reload rebuilds the route table from the provided routes.
func (r *Router) Reload(routes []config.Route) {
	exact := make(map[string]routeEntry)
	var prefixes []prefixEntry

	for _, route := range routes {
		entry := routeEntry{
			route:    route,
			upstream: route.Upstream,
		}
		for _, p := range route.Paths {
			switch p.Type {
			case "exact":
				key := routeKey(route.Host, p.Path)
				exact[key] = entry
			case "prefix":
				prefixes = append(prefixes, prefixEntry{
					host:   route.Host,
					prefix: p.Path,
					entry:  entry,
				})
			}
		}
	}

	// Sort prefixes by length descending (longest prefix match first)
	sortPrefixesByLength(prefixes)

	r.exact.Store(&exact)
	r.prefixes.Store(&prefixes)

	slog.Info("route table reloaded",
		slog.Int("exact_routes", len(exact)),
		slog.Int("prefix_routes", len(prefixes)),
	)
}

// Match finds the best matching route for a request.
// Returns the upstream name and whether a match was found.
func (r *Router) Match(req *http.Request) (string, bool) {
	host := req.Host
	// Strip port from host if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	path := req.URL.Path

	// Try exact match first (O(1))
	exact := *r.exact.Load()
	key := routeKey(host, path)
	if entry, ok := exact[key]; ok {
		return entry.upstream, true
	}
	// Also try without host for wildcard routes
	if host != "" {
		key = routeKey("", path)
		if entry, ok := exact[key]; ok {
			return entry.upstream, true
		}
	}

	// Try prefix match (longest match wins)
	prefixes := *r.prefixes.Load()
	for _, pe := range prefixes {
		if pe.host != "" && pe.host != host {
			continue
		}
		if strings.HasPrefix(path, pe.prefix) {
			return pe.entry.upstream, true
		}
	}

	return "", false
}

func routeKey(host, path string) string {
	return host + "|" + path
}

func sortPrefixesByLength(entries []prefixEntry) {
	// Simple insertion sort (route tables are typically small)
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && len(entries[j].prefix) > len(entries[j-1].prefix) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}
}
