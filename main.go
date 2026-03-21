// Uniproxy is a universal test proxy that health-checks configured dependencies
// using the dephealth SDK and exposes Prometheus metrics.
// It serves as a lightweight adapter between the dephealth SDK and Kubernetes,
// exposing health status via HTTP endpoints and Prometheus metrics.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	// Register built-in checker factories (HTTP, gRPC, Redis, Postgres, etc.)
	// via init() side-effects. Without this import, dephealth.New() would not
	// know how to create checkers for any dependency type.
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"

	"github.com/BigKAA/uniproxy/internal/config"
	"github.com/BigKAA/uniproxy/internal/logging"
	"github.com/BigKAA/uniproxy/internal/server"
)

// main is the application entry point. It performs the following steps:
//  1. Loads configuration from environment variables and/or YAML config file.
//  2. Initializes structured logging based on the loaded config.
//  3. Builds dephealth SDK options from the parsed dependency configurations.
//  4. Creates and starts the dephealth health checker (runs checks in background goroutines).
//  5. Creates and starts the HTTP server with auth middleware and route handlers.
//  6. Waits for SIGINT/SIGTERM, then performs graceful shutdown.
func main() {
	// Load configuration from environment variables and optional YAML file (CONFIG_FILE).
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set up structured logging and replace the default slog logger,
	// so all subsequent slog calls use the configured format and level.
	logger := logging.NewLogger(cfg.Log)
	slog.SetDefault(logger)

	slog.Info("config loaded",
		"name", cfg.Name,
		"group", cfg.Group,
		"listen", cfg.ListenAddr,
		"dependencies", len(cfg.Dependencies),
		"checkInterval", cfg.CheckInterval,
	)

	// Create a context that is cancelled on SIGINT or SIGTERM.
	// This context controls the lifecycle of the health checker and HTTP server.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Build the slice of dephealth SDK options from the application config.
	opts, err := buildOptions(cfg, logger)
	if err != nil {
		slog.Error("failed to build options", "error", err)
		os.Exit(1)
	}

	// Create the dephealth instance with the application name, group, and options.
	// This registers Prometheus metrics and prepares health checkers for each dependency.
	dh, err := dephealth.New(cfg.Name, cfg.Group, opts...)
	if err != nil {
		slog.Error("failed to create dephealth", "error", err)
		os.Exit(1)
	}

	// Start background health checks. The SDK will periodically check each
	// dependency and update Prometheus metrics (health, latency, status, status_detail).
	if err := dh.Start(ctx); err != nil {
		slog.Error("failed to start dephealth", "error", err)
		os.Exit(1)
	}
	slog.Info("dephealth started", "name", cfg.Name)

	// Configure HTTP transport and circuit breaker from settings.
	if cfg.HTTPTransport != nil || cfg.CircuitBreaker != nil {
		httpTransport := cfg.HTTPTransport
		cb := cfg.CircuitBreaker
		server.ConfigureFetch(
			httpTransport.MaxIdleConns,
			httpTransport.MaxIdleConnsPerHost,
			httpTransport.IdleConnTimeout,
			uint32(cb.MaxFailures), //nolint:gosec // validated to be positive in config
			cb.Timeout,
			uint32(cb.HalfOpenLimit), //nolint:gosec // validated to be positive in config
		)
	}

	// Resolve effective auth config for each zone and log it for observability.
	statusAuth := cfg.Auth.ResolveZone("status")
	metricsAuth := cfg.Auth.ResolveZone("metrics")
	slog.Info("auth config",
		"status_method", statusAuth.Method,
		"metrics_method", metricsAuth.Method,
	)

	// Create the HTTP server with Chi router, auth middleware, and route handlers.
	srv := server.New(dh, cfg.Name, cfg.FetchTimeout, cfg.Auth)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the HTTP server in a separate goroutine so the main goroutine
	// can block on the context cancellation signal.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Block until SIGINT/SIGTERM is received or server fails to start.
	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case err := <-serverErr:
		slog.Error("server error", "error", err)
	}

	// Graceful shutdown with timeout.
	// 1. Stop accepting new health check requests.
	// 2. Shutdown HTTP server, allowing active requests to complete.
	// 3. If timeout is exceeded, force shutdown.
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 30 * time.Second
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	slog.Info("stopping health checks")
	dh.Stop()

	slog.Info("shutting down HTTP server", "timeout", shutdownTimeout)
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Warn("HTTP server shutdown error", "error", err)
	} else {
		slog.Info("HTTP server stopped gracefully")
	}
}

// buildOptions creates the slice of dephealth SDK options from the application config.
// It sets global options (check interval, timeout, logger) and converts each
// configured dependency into a corresponding SDK dependency option.
//
// Parameters:
//   - cfg: the fully loaded and validated application configuration.
//   - logger: the configured slog.Logger to pass to the SDK for internal logging.
//
// Returns an error if any dependency option cannot be built (e.g. unsupported type).
func buildOptions(cfg *config.Config, logger *slog.Logger) ([]dephealth.Option, error) {
	opts := []dephealth.Option{
		dephealth.WithCheckInterval(cfg.CheckInterval),
		dephealth.WithLogger(logger),
	}
	// Global timeout is optional; 0 means the SDK will use its own default.
	if cfg.Timeout > 0 {
		opts = append(opts, dephealth.WithTimeout(cfg.Timeout))
	}

	// Convert each dependency config into an SDK dependency option.
	for _, dep := range cfg.Dependencies {
		opt, err := buildDependencyOption(dep, cfg.IsEntry)
		if err != nil {
			return nil, err
		}
		opts = append(opts, opt)
	}
	return opts, nil
}

// buildDependencyOption converts a single Dependency config struct into a dephealth
// SDK option by assembling the appropriate DependencyOption slice and calling the
// correct factory function (HTTP, Redis, Postgres, gRPC, TCP, MySQL, AMQP, Kafka, LDAP).
//
// The function handles:
//   - Connection source: URL-based or explicit host+port.
//   - Critical flag: marks the dependency as critical for readiness.
//   - IsEntry label: when isEntry is true, adds isentry=yes label for topology visualization.
//   - Per-dependency timing overrides (check interval, timeout).
//   - Type-specific options: TLS, health paths, service names, queries, passwords, LDAP config.
//   - Type-specific authentication: bearer tokens, basic auth, custom headers/metadata.
//
// Returns an error only if the dependency type is not supported.
func buildDependencyOption(dep config.Dependency, isEntry bool) (dephealth.Option, error) {
	var depOpts []dephealth.DependencyOption

	// Connection source: prefer URL if provided, otherwise use host+port pair.
	if dep.URL != "" {
		depOpts = append(depOpts, dephealth.FromURL(dep.URL))
	} else {
		depOpts = append(depOpts, dephealth.FromParams(dep.Host, dep.Port))
	}

	// Mark whether this dependency is critical for the application's readiness.
	depOpts = append(depOpts, dephealth.Critical(dep.Critical))

	// Add isentry label for entry-point applications in topology visualization.
	if isEntry {
		depOpts = append(depOpts, dephealth.WithLabel("isentry", "yes"))
	}

	// Per-dependency timing overrides; 0 means inherit from global settings.
	if dep.CheckInterval > 0 {
		depOpts = append(depOpts, dephealth.CheckInterval(dep.CheckInterval))
	}
	if dep.Timeout > 0 {
		depOpts = append(depOpts, dephealth.Timeout(dep.Timeout))
	}

	// Type-specific options and authentication.
	switch dep.Type {
	case "http":
		if dep.HealthPath != "" {
			depOpts = append(depOpts, dephealth.WithHTTPHealthPath(dep.HealthPath))
		}
		if dep.TLS != nil {
			depOpts = append(depOpts, dephealth.WithHTTPTLS(*dep.TLS))
		}
		if dep.TLSSkipVerify != nil {
			depOpts = append(depOpts, dephealth.WithHTTPTLSSkipVerify(*dep.TLSSkipVerify))
		}
		if dep.HostHeader != "" {
			depOpts = append(depOpts, dephealth.WithHTTPHostHeader(dep.HostHeader))
		}
		// HTTP authentication options.
		if dep.BearerToken != "" {
			depOpts = append(depOpts, dephealth.WithHTTPBearerToken(dep.BearerToken))
		}
		if dep.BasicUser != "" {
			depOpts = append(depOpts, dephealth.WithHTTPBasicAuth(dep.BasicUser, dep.BasicPass))
		}
		if len(dep.Headers) > 0 {
			depOpts = append(depOpts, dephealth.WithHTTPHeaders(dep.Headers))
		}
	case "grpc":
		if dep.GRPCServiceName != "" {
			depOpts = append(depOpts, dephealth.WithGRPCServiceName(dep.GRPCServiceName))
		}
		if dep.TLS != nil {
			depOpts = append(depOpts, dephealth.WithGRPCTLS(*dep.TLS))
		}
		if dep.TLSSkipVerify != nil {
			depOpts = append(depOpts, dephealth.WithGRPCTLSSkipVerify(*dep.TLSSkipVerify))
		}
		if dep.GRPCAuthority != "" {
			depOpts = append(depOpts, dephealth.WithGRPCAuthority(dep.GRPCAuthority))
		}
		// gRPC authentication options.
		if dep.BearerToken != "" {
			depOpts = append(depOpts, dephealth.WithGRPCBearerToken(dep.BearerToken))
		}
		if dep.BasicUser != "" {
			depOpts = append(depOpts, dephealth.WithGRPCBasicAuth(dep.BasicUser, dep.BasicPass))
		}
		if len(dep.Metadata) > 0 {
			depOpts = append(depOpts, dephealth.WithGRPCMetadata(dep.Metadata))
		}
	case "postgres":
		// Custom health-check query (default: SELECT 1).
		if dep.PostgresQuery != "" {
			depOpts = append(depOpts, dephealth.WithPostgresQuery(dep.PostgresQuery))
		}
	case "mysql":
		// Custom health-check query (default: SELECT 1).
		if dep.MySQLQuery != "" {
			depOpts = append(depOpts, dephealth.WithMySQLQuery(dep.MySQLQuery))
		}
	case "redis":
		if dep.RedisPassword != "" {
			depOpts = append(depOpts, dephealth.WithRedisPassword(dep.RedisPassword))
		}
		if dep.RedisDB != nil {
			depOpts = append(depOpts, dephealth.WithRedisDB(*dep.RedisDB))
		}
	case "amqp":
		// AMQP URL overrides host+port for connection string.
		if dep.AMQPURL != "" {
			depOpts = append(depOpts, dephealth.WithAMQPURL(dep.AMQPURL))
		}
	case "ldap":
		// LDAP check method determines how health is verified:
		// anonymous_bind, simple_bind, root_dse, or search.
		if dep.LDAPCheckMethod != "" {
			depOpts = append(depOpts, dephealth.WithLDAPCheckMethod(dep.LDAPCheckMethod))
		}
		if dep.LDAPBindDN != "" {
			depOpts = append(depOpts, dephealth.WithLDAPBindDN(dep.LDAPBindDN))
		}
		if dep.LDAPBindPassword != "" {
			depOpts = append(depOpts, dephealth.WithLDAPBindPassword(dep.LDAPBindPassword))
		}
		if dep.LDAPBaseDN != "" {
			depOpts = append(depOpts, dephealth.WithLDAPBaseDN(dep.LDAPBaseDN))
		}
		if dep.LDAPSearchFilter != "" {
			depOpts = append(depOpts, dephealth.WithLDAPSearchFilter(dep.LDAPSearchFilter))
		}
		if dep.LDAPSearchScope != "" {
			depOpts = append(depOpts, dephealth.WithLDAPSearchScope(dep.LDAPSearchScope))
		}
		if dep.LDAPStartTLS != nil {
			depOpts = append(depOpts, dephealth.WithLDAPStartTLS(*dep.LDAPStartTLS))
		}
		if dep.LDAPTLSSkipVerify != nil {
			depOpts = append(depOpts, dephealth.WithLDAPTLSSkipVerify(*dep.LDAPTLSSkipVerify))
		}
	}

	// Select the appropriate SDK factory by dependency type.
	// Each factory creates a checker that knows how to connect to and verify
	// the health of the specific dependency type.
	switch dep.Type {
	case "http":
		return dephealth.HTTP(dep.Name, depOpts...), nil
	case "redis":
		return dephealth.Redis(dep.Name, depOpts...), nil
	case "postgres":
		return dephealth.Postgres(dep.Name, depOpts...), nil
	case "grpc":
		return dephealth.GRPC(dep.Name, depOpts...), nil
	case "tcp":
		return dephealth.TCP(dep.Name, depOpts...), nil
	case "mysql":
		return dephealth.MySQL(dep.Name, depOpts...), nil
	case "amqp":
		return dephealth.AMQP(dep.Name, depOpts...), nil
	case "kafka":
		return dephealth.Kafka(dep.Name, depOpts...), nil
	case "ldap":
		return dephealth.LDAP(dep.Name, depOpts...), nil
	default:
		return nil, fmt.Errorf("unsupported dependency type %q for %q", dep.Type, dep.Name)
	}
}
