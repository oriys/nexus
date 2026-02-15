package config

import "time"

// Config is the top-level gateway configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Upstreams []Upstream      `yaml:"upstreams"`
	Routes    []Route         `yaml:"routes"`
	Logging   LoggingConfig   `yaml:"logging"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Auth      AuthConfig      `yaml:"auth"`
	Admin     AdminConfig     `yaml:"admin"`
}

// ServerConfig defines the HTTP server settings.
type ServerConfig struct {
	Listen          string        `yaml:"listen"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// Upstream defines a group of backend targets.
type Upstream struct {
	Name    string   `yaml:"name"`
	Targets []Target `yaml:"targets"`
}

// Target represents a single backend address.
type Target struct {
	Address string `yaml:"address"`
	Weight  int    `yaml:"weight"`
}

// Route maps incoming requests to an upstream.
type Route struct {
	Name     string        `yaml:"name"`
	Host     string        `yaml:"host"`
	Paths    []PathRule    `yaml:"paths"`
	Upstream string        `yaml:"upstream"`
	Rewrite  *RewriteRule  `yaml:"rewrite,omitempty"`
}

// RewriteRule defines request rewriting rules for a route.
type RewriteRule struct {
	// Protocol is the target backend protocol: "http" (default), "grpc", "dubbo".
	Protocol string `yaml:"protocol,omitempty"`

	// Path rewrites the request path. Supports prefix replacement.
	// Example: "/api/v1" → "/internal/v2"
	PathRewrite *PathRewrite `yaml:"path_rewrite,omitempty"`

	// Headers defines header manipulation rules.
	Headers *HeaderRewrite `yaml:"headers,omitempty"`

	// GRPC defines gRPC-specific rewrite settings (used when protocol is "grpc").
	GRPC *GRPCRewrite `yaml:"grpc,omitempty"`

	// Dubbo defines Dubbo-specific rewrite settings (used when protocol is "dubbo").
	Dubbo *DubboRewrite `yaml:"dubbo,omitempty"`
}

// PathRewrite defines path rewriting rules.
type PathRewrite struct {
	// Prefix replaces the matching path prefix with the given value.
	// For example, if the route matches "/api" and Prefix is "/internal",
	// then "/api/users" becomes "/internal/users".
	Prefix string `yaml:"prefix"`
}

// HeaderRewrite defines header manipulation rules.
type HeaderRewrite struct {
	// Add adds headers to the request (appends if exists).
	Add map[string]string `yaml:"add,omitempty"`
	// Set sets headers on the request (overwrites if exists).
	Set map[string]string `yaml:"set,omitempty"`
	// Remove removes headers from the request.
	Remove []string `yaml:"remove,omitempty"`
}

// GRPCRewrite defines gRPC-specific rewrite configuration.
type GRPCRewrite struct {
	// Service is the fully qualified gRPC service name (e.g., "helloworld.Greeter").
	Service string `yaml:"service"`
	// Method is the gRPC method name (e.g., "SayHello").
	Method string `yaml:"method"`
}

// DubboRewrite defines Dubbo-specific rewrite configuration.
type DubboRewrite struct {
	// Service is the Dubbo service interface (e.g., "com.example.UserService").
	Service string `yaml:"service"`
	// Method is the Dubbo method name (e.g., "getUser").
	Method string `yaml:"method"`
	// Group is the Dubbo service group.
	Group string `yaml:"group,omitempty"`
	// Version is the Dubbo service version.
	Version string `yaml:"version,omitempty"`
}

// PathRule defines a path matching rule.
type PathRule struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"` // "exact" or "prefix"
}

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// RateLimitConfig defines rate limiting settings.
type RateLimitConfig struct {
	Enabled bool          `yaml:"enabled"`
	Rate    int           `yaml:"rate"`
	Window  time.Duration `yaml:"window"`
}

// AuthConfig defines authentication settings.
type AuthConfig struct {
	APIKey APIKeyConfig `yaml:"api_key"`
}

// APIKeyConfig defines API key authentication settings.
type APIKeyConfig struct {
	Enabled bool              `yaml:"enabled"`
	Keys    map[string]string `yaml:"keys"` // key → consumer name
}

// AdminConfig defines admin API settings.
type AdminConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}
