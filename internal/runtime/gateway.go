package runtime

import (
	"log/slog"
	"net/http"
)

// Gateway is the main request handler that uses CompiledConfig for routing.
type Gateway struct {
	store      *ConfigStore
	dispatcher *UpstreamDispatcher
}

// NewGateway creates a new Gateway handler.
func NewGateway(store *ConfigStore) *Gateway {
	return &Gateway{
		store:      store,
		dispatcher: NewUpstreamDispatcher(),
	}
}

// ServeHTTP handles incoming requests using the compiled configuration.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := g.store.Load()
	if cfg == nil {
		http.Error(w, "gateway not configured", http.StatusServiceUnavailable)
		return
	}

	// Match route
	route, matched := cfg.Router.Match(r)
	if !matched {
		http.Error(w, "no matching route", http.StatusNotFound)
		return
	}

	// Apply filters
	for _, f := range route.Filters {
		if err := f.Apply(r); err != nil {
			slog.Error("filter error",
				slog.String("route", route.Name),
				slog.String("error", err.Error()),
			)
			http.Error(w, "filter error", http.StatusBadRequest)
			return
		}
	}

	// Find cluster
	cluster, ok := cfg.Clusters[route.Upstream.ClusterName]
	if !ok {
		slog.Error("cluster not found",
			slog.String("route", route.Name),
			slog.String("cluster", route.Upstream.ClusterName),
		)
		http.Error(w, "upstream not available", http.StatusBadGateway)
		return
	}

	// Dispatch to upstream
	if err := g.dispatcher.Dispatch(w, r, route, cluster); err != nil {
		slog.Error("upstream dispatch error",
			slog.String("route", route.Name),
			slog.String("cluster", cluster.Name),
			slog.String("error", err.Error()),
		)
		// The HTTP error response is written by the upstream's ErrorHandler
	}
}
