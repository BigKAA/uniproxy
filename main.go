// Uniproxy is a universal test proxy that health-checks configured dependencies
// using the dephealth SDK and exposes Prometheus metrics.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
	// Register built-in checker factories.
	_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"

	"github.com/BigKAA/uniproxy/internal/config"
	"github.com/BigKAA/uniproxy/internal/logging"
	"github.com/BigKAA/uniproxy/internal/server"
)

func main() {
	// Load configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set up logging.
	logger := logging.NewLogger(cfg.Log)
	slog.SetDefault(logger)

	slog.Info("config loaded",
		"name", cfg.Name,
		"listen", cfg.ListenAddr,
		"dependencies", len(cfg.Dependencies),
		"checkInterval", cfg.CheckInterval,
	)

	// Start health checks.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	opts, err := buildOptions(cfg, logger)
	if err != nil {
		slog.Error("failed to build options", "error", err)
		os.Exit(1)
	}

	dh, err := dephealth.New(cfg.Name, opts...)
	if err != nil {
		slog.Error("failed to create dephealth", "error", err)
		os.Exit(1)
	}

	if err := dh.Start(ctx); err != nil {
		slog.Error("failed to start dephealth", "error", err)
		os.Exit(1)
	}
	slog.Info("dephealth started", "name", cfg.Name)

	// Start HTTP server.
	srv := server.New(dh, cfg.Name, cfg.FetchTimeout)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv.Handler(),
	}

	go func() {
		slog.Info("server starting", "addr", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	dh.Stop()
	httpServer.Close()
}

// buildOptions creates dephealth SDK options from the application config.
func buildOptions(cfg *config.Config, logger *slog.Logger) ([]dephealth.Option, error) {
	opts := []dephealth.Option{
		dephealth.WithCheckInterval(cfg.CheckInterval),
		dephealth.WithLogger(logger),
	}
	if cfg.Timeout > 0 {
		opts = append(opts, dephealth.WithTimeout(cfg.Timeout))
	}

	for _, dep := range cfg.Dependencies {
		opt, err := buildDependencyOption(dep)
		if err != nil {
			return nil, err
		}
		opts = append(opts, opt)
	}
	return opts, nil
}

// buildDependencyOption creates a dephealth dependency option from config.
func buildDependencyOption(dep config.Dependency) (dephealth.Option, error) {
	var depOpts []dephealth.DependencyOption

	// Connection source: URL or explicit host+port.
	if dep.URL != "" {
		depOpts = append(depOpts, dephealth.FromURL(dep.URL))
	} else {
		depOpts = append(depOpts, dephealth.FromParams(dep.Host, dep.Port))
	}

	depOpts = append(depOpts, dephealth.Critical(dep.Critical))

	if dep.CheckInterval > 0 {
		depOpts = append(depOpts, dephealth.CheckInterval(dep.CheckInterval))
	}
	if dep.Timeout > 0 {
		depOpts = append(depOpts, dephealth.Timeout(dep.Timeout))
	}

	// Type-specific options.
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
		if dep.PostgresQuery != "" {
			depOpts = append(depOpts, dephealth.WithPostgresQuery(dep.PostgresQuery))
		}
	case "mysql":
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
		if dep.AMQPURL != "" {
			depOpts = append(depOpts, dephealth.WithAMQPURL(dep.AMQPURL))
		}
	}

	// Factory by type.
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
	default:
		return nil, fmt.Errorf("unsupported dependency type %q for %q", dep.Type, dep.Name)
	}
}
