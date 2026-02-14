package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nexus/internal/admin"
	"github.com/oriys/nexus/internal/auth"
	"github.com/oriys/nexus/internal/circuitbreaker"
	"github.com/oriys/nexus/internal/config"
	"github.com/oriys/nexus/internal/health"
	"github.com/oriys/nexus/internal/metrics"
	"github.com/oriys/nexus/internal/middleware"
	"github.com/oriys/nexus/internal/proxy"
	"github.com/oriys/nexus/internal/ratelimit"
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

	// Initialize config version manager
	versionMgr := config.NewVersionManager(10)
	rawData, err := os.ReadFile(configPath)
	if err != nil {
		slog.Warn("failed to read raw config for versioning", slog.String("error", err.Error()))
		rawData = nil
	}
	versionMgr.Save(cfg, rawData)

	// Initialize components
	router := proxy.NewRouter()
	upstreamMgr := proxy.NewUpstreamManager()

	// Apply configuration
	router.Reload(cfg.Routes)
	upstreamMgr.Reload(cfg.Upstreams)

	// Health checker
	checker := health.NewChecker()

	// Build middleware chain
	middlewares := []middleware.Middleware{
		middleware.RequestID(),
		middleware.TraceContext(),
		middleware.Logging(),
	}

	// Add metrics middleware if enabled
	if cfg.Metrics.Enabled {
		middlewares = append(middlewares, middleware.Metrics())
		slog.Info("prometheus metrics enabled")
	}

	// Add rate limiting middleware if enabled
	if cfg.RateLimit.Enabled && cfg.RateLimit.Rate > 0 {
		window := cfg.RateLimit.Window
		if window == 0 {
			window = time.Minute
		}
		limiter := ratelimit.NewLimiter(cfg.RateLimit.Rate, window)
		middlewares = append(middlewares, middleware.RateLimit(limiter, middleware.ClientIPKeyExtractor))
		slog.Info("rate limiting enabled",
			slog.Int("rate", cfg.RateLimit.Rate),
			slog.Duration("window", window),
		)
	}

	// Add auth middleware if enabled
	if cfg.Auth.APIKey.Enabled && len(cfg.Auth.APIKey.Keys) > 0 {
		authenticator := auth.NewAPIKeyAuthenticator(cfg.Auth.APIKey.Keys)
		middlewares = append(middlewares, middleware.Auth(authenticator))
		slog.Info("API key authentication enabled",
			slog.Int("keys", len(cfg.Auth.APIKey.Keys)),
		)
	}

	// Build handler with middleware chain
	proxyHandler := proxy.NewProxy(router, upstreamMgr)

	// Set up circuit breakers if enabled
	if cfg.CircuitBreaker.Enabled {
		failureThreshold := cfg.CircuitBreaker.FailureThreshold
		if failureThreshold == 0 {
			failureThreshold = 5
		}
		successThreshold := cfg.CircuitBreaker.SuccessThreshold
		if successThreshold == 0 {
			successThreshold = 2
		}
		timeout := cfg.CircuitBreaker.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		breakers := make(map[string]*circuitbreaker.CircuitBreaker)
		for _, u := range cfg.Upstreams {
			breakers[u.Name] = circuitbreaker.New(u.Name, failureThreshold, successThreshold, timeout)
		}
		proxyHandler.SetCircuitBreakers(breakers)
		slog.Info("circuit breaker enabled",
			slog.Int("failure_threshold", failureThreshold),
			slog.Int("success_threshold", successThreshold),
			slog.Duration("timeout", timeout),
		)
	}

	handler := middleware.Chain(proxyHandler, middlewares...)

	// Create mux with health and metrics endpoints
	mux := http.NewServeMux()
	mux.Handle("/healthz", checker.HealthzHandler())
	mux.Handle("/readyz", checker.ReadyzHandler())
	if cfg.Metrics.Enabled {
		metricsPath := cfg.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.Handle(metricsPath, metrics.Handler())
	}
	mux.Handle("/", handler)

	// Configure server
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start TLS server if enabled
	var tlsSrv *http.Server
	if cfg.Server.TLS.Enabled && cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "" {
		tlsListen := cfg.Server.TLS.Listen
		if tlsListen == "" {
			tlsListen = ":8443"
		}
		tlsSrv = &http.Server{
			Addr:         tlsListen,
			Handler:      mux,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
		go func() {
			slog.Info("HTTPS server starting",
				slog.String("listen", tlsListen),
				slog.String("cert", cfg.Server.TLS.CertFile),
			)
			if err := tlsSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				slog.Error("TLS server error", slog.String("error", err.Error()))
			}
		}()
	}

	// Start admin API server if enabled
	var adminSrv *http.Server
	if cfg.Admin.Enabled && cfg.Admin.Listen != "" {
		adminServer := admin.New(loader, versionMgr, router, upstreamMgr)
		adminSrv = &http.Server{
			Addr:    cfg.Admin.Listen,
			Handler: adminServer.Handler(),
		}
		go func() {
			slog.Info("admin API starting", slog.String("listen", cfg.Admin.Listen))
			if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("admin server error", slog.String("error", err.Error()))
			}
		}()
	}

	// Start config watcher
	done := make(chan struct{})
	go func() {
		if err := loader.Watch(func(newCfg *config.Config) {
			router.Reload(newCfg.Routes)
			upstreamMgr.Reload(newCfg.Upstreams)
			newRawData, err := os.ReadFile(configPath)
			if err != nil {
				slog.Warn("failed to read raw config for versioning", slog.String("error", err.Error()))
				newRawData = nil
			}
			versionMgr.Save(newCfg, newRawData)
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
		shutdownTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if tlsSrv != nil {
		if err := tlsSrv.Shutdown(ctx); err != nil {
			slog.Error("TLS shutdown error", slog.String("error", err.Error()))
		}
	}

	if adminSrv != nil {
		if err := adminSrv.Shutdown(ctx); err != nil {
			slog.Error("admin shutdown error", slog.String("error", err.Error()))
		}
	}

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("nexus gateway stopped")
}
