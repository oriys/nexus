package plugin

import (
	"log/slog"
	"time"
)

// GlobalLogPlugin logs request details before and after the downstream
// plugins execute, including latency measurement.
type GlobalLogPlugin struct{}

// NewGlobalLogPlugin creates a new GlobalLogPlugin.
func NewGlobalLogPlugin() *GlobalLogPlugin {
	return &GlobalLogPlugin{}
}

// Name returns the plugin name.
func (p *GlobalLogPlugin) Name() string { return "global_log" }

// Order returns the execution order. GlobalLogPlugin runs first (order 0)
// so it can wrap the entire chain for timing.
func (p *GlobalLogPlugin) Order() int { return 0 }

// Execute logs the incoming request, calls the next plugin, then logs the
// completion with latency.
func (p *GlobalLogPlugin) Execute(ctx *GatewayContext, next func()) error {
	start := time.Now()
	r := ctx.Request

	slog.Info("plugin request begin",
		slog.String("plugin", p.Name()),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("host", r.Host),
		slog.String("remote_addr", r.RemoteAddr),
	)

	next()

	slog.Info("plugin request end",
		slog.String("plugin", p.Name()),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Duration("latency", time.Since(start)),
	)

	return nil
}
