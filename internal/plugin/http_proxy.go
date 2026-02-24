package plugin

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// HttpProxyPlugin forwards the request to the upstream backend using
// httputil.ReverseProxy. It reads the upstream target from GatewayContext.Rule.
type HttpProxyPlugin struct{}

// NewHttpProxyPlugin creates a new HttpProxyPlugin.
func NewHttpProxyPlugin() *HttpProxyPlugin {
	return &HttpProxyPlugin{}
}

// Name returns the plugin name.
func (p *HttpProxyPlugin) Name() string { return "http_proxy" }

// Order returns the execution order. HttpProxyPlugin runs last (order 100)
// since it is the terminal plugin that performs the actual proxying.
func (p *HttpProxyPlugin) Order() int { return 100 }

// Execute performs the HTTP reverse proxy. It does NOT call next() because
// it is the terminal plugin in the chain.
func (p *HttpProxyPlugin) Execute(ctx *GatewayContext, next func()) error {
	if ctx.Rule == nil || ctx.Rule.Upstream == "" {
		http.Error(ctx.ResponseWriter, "no upstream configured", http.StatusBadGateway)
		return nil
	}

	upstream := ctx.Rule.Upstream

	// Parse upstream as URL; prepend scheme if missing.
	targetURL, err := url.Parse(upstream)
	if err != nil || targetURL.Host == "" {
		targetURL, err = url.Parse("http://" + upstream)
		if err != nil {
			slog.Error("invalid upstream target",
				slog.String("plugin", p.Name()),
				slog.String("upstream", upstream),
				slog.String("error", err.Error()),
			)
			http.Error(ctx.ResponseWriter, "bad gateway", http.StatusBadGateway)
			return nil
		}
	}

	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.Out.Host = ctx.Request.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error",
				slog.String("plugin", p.Name()),
				slog.String("upstream", upstream),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	rp.ServeHTTP(ctx.ResponseWriter, ctx.Request)

	// Terminal plugin — do not call next().
	return nil
}
