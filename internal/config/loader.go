package config

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Loader handles loading and hot-reloading of gateway configuration.
type Loader struct {
	path    string
	current atomic.Value // stores *Config
}

// NewLoader creates a new configuration loader for the given file path.
func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

// Load reads and parses the configuration file.
func (l *Loader) Load() (*Config, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	l.current.Store(&cfg)
	return &cfg, nil
}

// Current returns the currently loaded configuration.
func (l *Loader) Current() *Config {
	v := l.current.Load()
	if v == nil {
		return nil
	}
	return v.(*Config)
}

// Watch starts watching the configuration file for changes and calls onChange
// when the file is modified. It blocks until the done channel is closed.
func (l *Loader) Watch(onChange func(*Config), done <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create file watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(l.path); err != nil {
		return fmt.Errorf("watch config file: %w", err)
	}

	slog.Info("watching config file for changes", slog.String("path", l.path))

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				slog.Info("config file changed, reloading", slog.String("path", l.path))
				cfg, err := l.Load()
				if err != nil {
					slog.Error("failed to reload config, keeping current",
						slog.String("error", err.Error()),
					)
					continue
				}
				if onChange != nil {
					onChange(cfg)
				}
				slog.Info("config reloaded successfully")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("config watcher error", slog.String("error", err.Error()))
		case <-done:
			return nil
		}
	}
}
