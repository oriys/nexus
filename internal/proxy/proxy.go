package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	upstreamName, matched := p.router.Match(r)
	if !matched {
		http.Error(w, "no matching route", http.StatusNotFound)
		return
	}

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
