// Package plugin implements a ShenYu-style plugin responsibility chain
// for the gateway. Each plugin can inspect, modify, or short-circuit
// requests as they flow through the chain.
package plugin

import (
	"net/http"
	"sort"
)

// RuleData holds the matched route and rule information for a request.
type RuleData struct {
	// Name is the name of the matched route/rule.
	Name string
	// Upstream is the target upstream name.
	Upstream string
	// PathPrefix is the matched path prefix.
	PathPrefix string
	// Host is the matched host.
	Host string
}

// GatewayContext carries all context information for a single request
// through the plugin chain.
type GatewayContext struct {
	Request        *http.Request
	ResponseWriter http.ResponseWriter
	// Attributes stores data passed between plugins.
	Attributes map[string]interface{}
	// Rule is the matched routing rule.
	Rule *RuleData
}

// Plugin defines the interface that all gateway plugins must implement.
type Plugin interface {
	// Name returns the plugin name (e.g. "global_log", "http_proxy").
	Name() string
	// Order returns the execution order. Lower values execute first.
	Order() int
	// Execute runs the plugin logic. Call next() to pass control to the
	// next plugin in the chain. Returning an error short-circuits further
	// processing.
	Execute(ctx *GatewayContext, next func()) error
}

// Chain holds an ordered list of plugins and executes them as a
// responsibility chain.
type Chain struct {
	plugins []Plugin
}

// NewChain creates a new plugin chain. Plugins are sorted by Order()
// (ascending) so the lowest order executes first.
func NewChain(plugins ...Plugin) *Chain {
	sorted := make([]Plugin, len(plugins))
	copy(sorted, plugins)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Order() < sorted[j].Order()
	})
	return &Chain{plugins: sorted}
}

// Execute runs the plugin chain for the given context.
func (c *Chain) Execute(ctx *GatewayContext) error {
	var chainErr error
	index := 0

	var run func()
	run = func() {
		if index >= len(c.plugins) {
			return
		}
		p := c.plugins[index]
		index++
		if err := p.Execute(ctx, run); err != nil {
			chainErr = err
		}
	}
	run()

	return chainErr
}

// Handler returns an http.Handler that creates a GatewayContext for each
// request and runs it through the plugin chain.
func (c *Chain) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := &GatewayContext{
			Request:        r,
			ResponseWriter: w,
			Attributes:     make(map[string]interface{}),
		}
		if err := c.Execute(ctx); err != nil {
			// If no response was written yet, return 500.
			http.Error(w, "internal plugin error", http.StatusInternalServerError)
		}
	})
}
