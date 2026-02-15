package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/oriys/nexus/internal/config"
)

// Proxy is the main reverse proxy handler that routes requests to upstreams.
type Proxy struct {
	router   *Router
	upstream *UpstreamManager
}

// NewProxy creates a new reverse proxy handler.
func NewProxy(router *Router, upstream *UpstreamManager) *Proxy {
	return &Proxy{
		router:   router,
		upstream: upstream,
	}
}

// ServeHTTP implements the http.Handler interface.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result, matched := p.router.Match(r)
	if !matched {
		http.Error(w, "no matching route", http.StatusNotFound)
		return
	}

	upstreamName := result.Upstream
	targetAddr, ok := p.upstream.GetTarget(upstreamName)
	if !ok {
		slog.Error("upstream not found", slog.String("upstream", upstreamName))
		http.Error(w, "upstream not available", http.StatusBadGateway)
		return
	}

	target, err := url.Parse("http://" + targetAddr)
	if err != nil {
		slog.Error("invalid upstream target",
			slog.String("target", targetAddr),
			slog.String("error", err.Error()),
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	// Determine the matched path prefix for path rewriting
	matchedPath := findMatchedPath(result.Route, r.URL.Path)

	// Apply request rewriting before proxying
	if err := ApplyRewrite(r, result.Route, matchedPath); err != nil {
		slog.Error("request rewrite failed",
			slog.String("route", result.Route.Name),
			slog.String("error", err.Error()),
		)
		http.Error(w, "request rewrite failed", http.StatusBadRequest)
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error",
				slog.String("upstream", upstreamName),
				slog.String("target", targetAddr),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

// findMatchedPath finds the path prefix that matched the route for path rewriting.
func findMatchedPath(route config.Route, requestPath string) string {
	for _, p := range route.Paths {
		switch p.Type {
		case "exact":
			if requestPath == p.Path {
				return p.Path
			}
		case "prefix":
			if strings.HasPrefix(requestPath, p.Path) {
				return p.Path
			}
		}
	}
	return ""
}
