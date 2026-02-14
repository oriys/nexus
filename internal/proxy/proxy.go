package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/oriys/nexus/internal/circuitbreaker"
)

// Proxy is the main reverse proxy handler that routes requests to upstreams.
type Proxy struct {
	router   *Router
	upstream *UpstreamManager
	breakers map[string]*circuitbreaker.CircuitBreaker
}

// NewProxy creates a new reverse proxy handler.
func NewProxy(router *Router, upstream *UpstreamManager) *Proxy {
	return &Proxy{
		router:   router,
		upstream: upstream,
	}
}

// SetCircuitBreakers sets circuit breakers for upstream names.
func (p *Proxy) SetCircuitBreakers(breakers map[string]*circuitbreaker.CircuitBreaker) {
	p.breakers = breakers
}

// ServeHTTP implements the http.Handler interface.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upstreamName, matched := p.router.Match(r)
	if !matched {
		http.Error(w, "no matching route", http.StatusNotFound)
		return
	}

	// Check circuit breaker if configured
	if p.breakers != nil {
		if cb, ok := p.breakers[upstreamName]; ok {
			if !cb.Allow() {
				slog.Warn("circuit breaker open",
					slog.String("upstream", upstreamName),
				)
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
				return
			}
		}
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

	cbForUpstream := p.getCircuitBreaker(upstreamName)

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
			if cbForUpstream != nil {
				cbForUpstream.RecordFailure()
			}
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			if cbForUpstream != nil {
				if resp.StatusCode >= 500 {
					cbForUpstream.RecordFailure()
				} else {
					cbForUpstream.RecordSuccess()
				}
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
}

func (p *Proxy) getCircuitBreaker(upstream string) *circuitbreaker.CircuitBreaker {
	if p.breakers == nil {
		return nil
	}
	return p.breakers[upstream]
}
