package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/oriys/nexus/internal/config"
	"github.com/oriys/nexus/internal/health"
	"github.com/oriys/nexus/internal/middleware"
	"github.com/oriys/nexus/internal/proxy"
)

func main() {
	// Initialize structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Determine config path
	configPath := os.Getenv("NEXUS_CONFIG")
	if configPath == "" {
		configPath = "configs/nexus.yaml"
	}

	// Load configuration
	loader := config.NewLoader(configPath)
	cfg, err := loader.Load()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("configuration loaded", slog.String("path", configPath))

	// Initialize components
	router := proxy.NewRouter()
	upstreamMgr := proxy.NewUpstreamManager()

	// Apply configuration
	router.Reload(cfg.Routes)
	upstreamMgr.Reload(cfg.Upstreams)

	// Health checker
	checker := health.NewChecker()

	// Build handler with middleware chain
	proxyHandler := proxy.NewProxy(router, upstreamMgr)
	handler := middleware.Chain(proxyHandler,
		middleware.RequestID(),
		middleware.Logging(),
	)

	// Create mux with health endpoints
	mux := http.NewServeMux()
	mux.Handle("/healthz", checker.HealthzHandler())
	mux.Handle("/readyz", checker.ReadyzHandler())
	mux.Handle("/", handler)

	// Configure server
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start config watcher
	done := make(chan struct{})
	go func() {
		if err := loader.Watch(func(newCfg *config.Config) {
			router.Reload(newCfg.Routes)
			upstreamMgr.Reload(newCfg.Upstreams)
		}, done); err != nil {
			slog.Error("config watcher error", slog.String("error", err.Error()))
		}
	}()

	// Start server
	go func() {
		slog.Info("nexus gateway starting", slog.String("listen", cfg.Server.Listen))
		checker.SetReady(true)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutdown signal received", slog.String("signal", sig.String()))

	// Graceful shutdown
	checker.SetReady(false)
	close(done) // stop config watcher

	shutdownTimeout := cfg.Server.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 30 * 1e9 // 30 seconds default
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("nexus gateway stopped")
}
