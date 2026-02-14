package config

import "time"

// Config is the top-level gateway configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Upstreams []Upstream      `yaml:"upstreams"`
	Routes    []Route         `yaml:"routes"`
	Logging   LoggingConfig   `yaml:"logging"`
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
	Name     string     `yaml:"name"`
	Host     string     `yaml:"host"`
	Paths    []PathRule `yaml:"paths"`
	Upstream string     `yaml:"upstream"`
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
