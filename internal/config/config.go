// Package config provides environment variable and YAML configuration parsing for uniproxy.
//
// Configuration is loaded in a layered manner:
//  1. Base config from an optional YAML file (CONFIG_FILE env var).
//  2. Environment variable overrides (env vars always take priority over YAML).
//  3. Defaults for any fields not set by either source.
//  4. Validation of the final configuration.
//
// The package supports two main configuration domains:
//   - Application config: name, group, check intervals, dependencies.
//   - Server auth config: zone-based authentication for HTTP endpoints.
//
// Secret values support a _FILE suffix pattern (e.g., AUTH_PASS_FILE) for
// reading secrets from files, which is useful in Kubernetes Secret volume mounts.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the top-level application configuration. It aggregates all settings
// required to run uniproxy: application identity, logging, server auth,
// health-check timing, resilience settings, and the list of dependencies to monitor.
type Config struct {
	Name            string                // application name for metrics labels and status responses
	Group           string                // logical group for metrics label (e.g. "backend", "infra")
	ListenAddr      string                // HTTP server listen address (default ":8080")
	Log             LogConfig             // structured logging configuration
	Auth            AuthConfig            // server-side (incoming) authentication for endpoints
	CheckInterval   time.Duration         // how often the SDK checks each dependency (default 10s)
	Timeout         time.Duration         // global check timeout (0 = SDK default)
	FetchTimeout    time.Duration         // timeout for recursive HTTP fetch in detail mode (default 5s)
	IsEntry         bool                  // when true, isentry=yes label is added to all dependency metrics
	Dependencies    []Dependency          // list of dependencies to health-check
	ShutdownTimeout time.Duration         // graceful shutdown timeout (default 30s)
	CircuitBreaker  *CircuitBreakerConfig // circuit breaker settings for downstream HTTP fetches
	HTTPTransport   *HTTPTransportConfig  // HTTP client transport settings for connection pooling
}

// CircuitBreakerConfig configures the circuit breaker pattern for downstream HTTP fetches.
type CircuitBreakerConfig struct {
	MaxFailures   int           // number of consecutive failures before opening circuit (default 5)
	Timeout       time.Duration // time in open state before transitioning to half-open (default 60s)
	HalfOpenLimit int           // number of requests allowed in half-open state (default 3)
}

// HTTPTransportConfig configures the HTTP client transport for downstream fetches.
type HTTPTransportConfig struct {
	MaxIdleConns        int           // maximum idle connections across all hosts (default 100)
	MaxIdleConnsPerHost int           // maximum idle connections per host (default 10)
	IdleConnTimeout     time.Duration // idle connection timeout (default 90s)
}

// AuthConfig describes server-side (incoming) authentication for protected endpoints.
// It uses a zone-based model where different HTTP endpoint groups ("status" for /,
// "metrics" for /metrics) can have independent auth configurations.
// Health probes (/healthz, /readyz) are always unauthenticated.
type AuthConfig struct {
	// Global defaults applied to all protected zones unless overridden.
	Method   string // "none" | "basic" | "bearer" | "apikey"
	Username string // username for basic auth
	Password string // password for basic auth
	Token    string // token for bearer auth
	APIKey   string // key for API key auth

	// Per-zone overrides. If non-nil, the zone's fields take priority over globals.
	Status  *ZoneAuth // override for the status zone (GET /)
	Metrics *ZoneAuth // override for the metrics zone (GET /metrics)
}

// ZoneAuth is a per-zone auth override. When set on AuthConfig.Status or
// AuthConfig.Metrics, its non-empty fields override the corresponding global values.
type ZoneAuth struct {
	Method   string
	Username string
	Password string
	Token    string
	APIKey   string
}

// ResolveZone returns the effective auth configuration for a named zone.
// Resolution order: zone-specific value > global value > "none" (default).
//
// Supported zone names: "status" (for GET /), "metrics" (for GET /metrics).
// Unknown zone names return global defaults with "none" fallback.
func (c *AuthConfig) ResolveZone(zone string) ZoneAuth {
	// Look up the zone-specific override pointer.
	var za *ZoneAuth
	switch zone {
	case "status":
		za = c.Status
	case "metrics":
		za = c.Metrics
	}

	// Start with global values as the base.
	result := ZoneAuth{
		Method:   c.Method,
		Username: c.Username,
		Password: c.Password,
		Token:    c.Token,
		APIKey:   c.APIKey,
	}

	// Override with zone-specific values where present.
	if za != nil {
		if za.Method != "" {
			result.Method = za.Method
		}
		if za.Username != "" {
			result.Username = za.Username
		}
		if za.Password != "" {
			result.Password = za.Password
		}
		if za.Token != "" {
			result.Token = za.Token
		}
		if za.APIKey != "" {
			result.APIKey = za.APIKey
		}
	}

	// Ensure method is never empty — default to "none" (no auth required).
	if result.Method == "" {
		result.Method = "none"
	}
	return result
}

// LogConfig holds logging configuration parsed from LOG_* environment variables.
// It controls output format, verbosity level, timestamp formatting, and
// custom JSON key names for structured logging.
type LogConfig struct {
	Format     string // "text" (default) or "json"
	Level      string // "debug", "info" (default), "warn", "error"
	TimeFormat string // "rfc3339", "rfc3339nano" (default), "unix", "unixmilli"
	AddSource  bool   // include file:line in log output (default: false)
	TimeKey    string // JSON key for timestamp (empty = slog default "time")
	LevelKey   string // JSON key for level (empty = slog default "level")
	MessageKey string // JSON key for message (empty = slog default "msg")
	SourceKey  string // JSON key for source (empty = slog default "source")
}

// Dependency describes a single dependency to health-check via the dephealth SDK.
// Each dependency has a type (http, redis, postgres, etc.) that determines which
// checker factory the SDK uses and which type-specific options are applicable.
type Dependency struct {
	Name       string // unique dependency name used as a metrics label
	Type       string // checker type: http, redis, postgres, grpc, tcp, mysql, amqp, kafka, ldap
	URL        string // connection URL (alternative to Host+Port)
	Host       string // hostname or IP (used with Port when URL is empty)
	Port       string // port number (used with Host when URL is empty)
	Critical   bool   // if true, dependency failure affects readiness
	HealthPath string // HTTP-specific: custom health endpoint path

	// Per-dependency timing overrides (0 = use global values).
	CheckInterval time.Duration
	Timeout       time.Duration

	// TLS options for HTTP and gRPC types. nil means use checker defaults.
	TLS           *bool // enable TLS
	TLSSkipVerify *bool // skip TLS certificate verification

	// HTTP-specific: overrides Host header (and TLS SNI when HTTPS).
	// Useful when health-checking services behind ingress/gateway by IP address.
	HostHeader string

	// gRPC-specific: target service name for gRPC health check protocol.
	GRPCServiceName string

	// gRPC-specific: overrides :authority pseudo-header (and TLS SNI when TLS).
	// Useful for gRPC services behind reverse proxy.
	GRPCAuthority string

	// Database-specific: custom health-check queries (default: SELECT 1).
	PostgresQuery string
	MySQLQuery    string

	// Redis-specific authentication and database selection.
	RedisPassword string
	RedisDB       *int // nil means default database (0)

	// AMQP-specific: full connection URL with credentials.
	AMQPURL string

	// LDAP-specific options for health check verification.
	LDAPCheckMethod   string // check method: "anonymous_bind", "simple_bind", "root_dse", "search"
	LDAPBindDN        string // distinguished name for simple_bind authentication
	LDAPBindPassword  string // password for simple_bind authentication
	LDAPBaseDN        string // base DN for LDAP search operations
	LDAPSearchFilter  string // LDAP filter for search method (default: (objectClass=*))
	LDAPSearchScope   string // search scope: "base", "one", "sub" (default: base)
	LDAPStartTLS      *bool  // enable StartTLS upgrade for ldap:// connections
	LDAPTLSSkipVerify *bool  // skip TLS certificate verification for LDAP

	// Authentication for outgoing health check requests.
	// Only one method per dependency is allowed (validated by validateAuth).
	BearerToken string            // bearer token for HTTP/gRPC auth
	BasicUser   string            // username for HTTP/gRPC basic auth
	BasicPass   string            // password for HTTP/gRPC basic auth
	Headers     map[string]string // HTTP-only: custom request headers
	Metadata    map[string]string // gRPC-only: custom call metadata
}

// Load parses configuration from YAML file (optional) and environment variables.
// Environment variables always take priority over YAML values.
//
// Loading flow:
//  1. If CONFIG_FILE env var is set, load base config from that YAML file.
//  2. Apply environment variable overrides (DEPHEALTH_*, LOG_*, AUTH_*, LISTEN_ADDR).
//  3. Fill in defaults for any fields still unset.
//  4. Validate the final configuration for correctness and completeness.
//
// Returns an error if the YAML file is invalid, env vars have bad values,
// required fields are missing, or auth config is inconsistent.
func Load() (*Config, error) {
	var cfg *Config

	// Phase 1: Base config from YAML (if provided via CONFIG_FILE env var).
	if path := os.Getenv("CONFIG_FILE"); path != "" {
		var err error
		cfg, err = loadFromYAML(path)
		if err != nil {
			return nil, fmt.Errorf("config file %q: %w", path, err)
		}
	} else {
		cfg = &Config{}
	}

	// Phase 2: Apply env var overrides (env > YAML).
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}

	// Phase 3: Apply defaults for unset fields.
	applyDefaults(cfg)

	// Phase 4: Validate the fully assembled config.
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides reads environment variables and overwrites the corresponding
// Config fields. Fields not set via env vars are left unchanged, preserving
// any values loaded from the YAML config file.
//
// This function handles:
//   - Core settings: LISTEN_ADDR, DEPHEALTH_NAME, DEPHEALTH_GROUP.
//   - Timing: DEPHEALTH_CHECK_INTERVAL, DEPHEALTH_TIMEOUT, DEPHEALTH_FETCH_TIMEOUT (in seconds).
//   - Global entry-point label: DEPHEALTH_ISENTRY.
//   - Server-side auth: AUTH_METHOD, AUTH_USER, AUTH_PASS, AUTH_TOKEN, AUTH_API_KEY.
//   - Dependencies: DEPHEALTH_DEPS (replaces all YAML deps if set).
//   - Logging: LOG_FORMAT, LOG_LEVEL, LOG_TIME_FORMAT, etc.
func applyEnvOverrides(cfg *Config) error {
	// Listen address.
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	// Log config overlay.
	if err := applyLogEnvOverrides(&cfg.Log); err != nil {
		return err
	}

	// Application name (required, but validated later).
	if v := os.Getenv("DEPHEALTH_NAME"); v != "" {
		cfg.Name = v
	}

	// Application group (required, but validated later).
	if v := os.Getenv("DEPHEALTH_GROUP"); v != "" {
		cfg.Group = v
	}

	// Check interval: parsed as floating-point seconds (e.g. "10" = 10s, "0.5" = 500ms).
	if v := os.Getenv("DEPHEALTH_CHECK_INTERVAL"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_CHECK_INTERVAL %q: %w", v, err)
		}
		cfg.CheckInterval = time.Duration(sec * float64(time.Second))
	}

	// Global timeout: optional; 0 means SDK uses its own default.
	if v := os.Getenv("DEPHEALTH_TIMEOUT"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_TIMEOUT %q: %w", v, err)
		}
		cfg.Timeout = time.Duration(sec * float64(time.Second))
	}

	// Fetch timeout for recursive HTTP detail requests.
	if v := os.Getenv("DEPHEALTH_FETCH_TIMEOUT"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_FETCH_TIMEOUT %q: %w", v, err)
		}
		cfg.FetchTimeout = time.Duration(sec * float64(time.Second))
	}

	// IsEntry flag: marks this application as an entry point in topology visualization.
	if v := os.Getenv("DEPHEALTH_ISENTRY"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_ISENTRY: %w", err)
		}
		cfg.IsEntry = b
	}

	// Graceful shutdown timeout.
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid SHUTDOWN_TIMEOUT %q: %w", v, err)
		}
		cfg.ShutdownTimeout = d
	}

	// Circuit breaker settings.
	if v := os.Getenv("CIRCUIT_BREAKER_MAX_FAILURES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid CIRCUIT_BREAKER_MAX_FAILURES %q: %w", v, err)
		}
		if cfg.CircuitBreaker == nil {
			cfg.CircuitBreaker = &CircuitBreakerConfig{}
		}
		cfg.CircuitBreaker.MaxFailures = n
	}
	if v := os.Getenv("CIRCUIT_BREAKER_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid CIRCUIT_BREAKER_TIMEOUT %q: %w", v, err)
		}
		if cfg.CircuitBreaker == nil {
			cfg.CircuitBreaker = &CircuitBreakerConfig{}
		}
		cfg.CircuitBreaker.Timeout = d
	}
	if v := os.Getenv("CIRCUIT_BREAKER_HALF_OPEN_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid CIRCUIT_BREAKER_HALF_OPEN_LIMIT %q: %w", v, err)
		}
		if cfg.CircuitBreaker == nil {
			cfg.CircuitBreaker = &CircuitBreakerConfig{}
		}
		cfg.CircuitBreaker.HalfOpenLimit = n
	}

	// HTTP transport settings.
	if v := os.Getenv("HTTP_TRANSPORT_MAX_IDLE_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid HTTP_TRANSPORT_MAX_IDLE_CONNS %q: %w", v, err)
		}
		if cfg.HTTPTransport == nil {
			cfg.HTTPTransport = &HTTPTransportConfig{}
		}
		cfg.HTTPTransport.MaxIdleConns = n
	}
	if v := os.Getenv("HTTP_TRANSPORT_MAX_IDLE_CONNS_PER_HOST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid HTTP_TRANSPORT_MAX_IDLE_CONNS_PER_HOST %q: %w", v, err)
		}
		if cfg.HTTPTransport == nil {
			cfg.HTTPTransport = &HTTPTransportConfig{}
		}
		cfg.HTTPTransport.MaxIdleConnsPerHost = n
	}
	if v := os.Getenv("HTTP_TRANSPORT_IDLE_CONN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid HTTP_TRANSPORT_IDLE_CONN_TIMEOUT %q: %w", v, err)
		}
		if cfg.HTTPTransport == nil {
			cfg.HTTPTransport = &HTTPTransportConfig{}
		}
		cfg.HTTPTransport.IdleConnTimeout = d
	}

	// Server-side (incoming) auth for status and metrics endpoints.
	if err := loadServerAuth(cfg); err != nil {
		return err
	}

	// Global auth defaults that are inherited by all dependencies unless overridden.
	ga, err := loadGlobalAuth()
	if err != nil {
		return err
	}

	// Dependencies: DEPHEALTH_DEPS completely replaces any YAML-defined deps.
	// If not set but YAML deps exist, only per-dep auth env var overlays are applied.
	if depsStr := os.Getenv("DEPHEALTH_DEPS"); depsStr != "" {
		deps, err := parseDeps(depsStr, ga)
		if err != nil {
			return err
		}
		cfg.Dependencies = deps
	} else if len(cfg.Dependencies) > 0 {
		// Apply per-dep auth env var overlays to existing (YAML-loaded) deps.
		for i := range cfg.Dependencies {
			prefix := "DEPHEALTH_" + EnvName(cfg.Dependencies[i].Name) + "_"
			if err := loadDepAuth(&cfg.Dependencies[i], prefix, ga); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyLogEnvOverrides reads LOG_* environment variables and overwrites the
// corresponding LogConfig fields. Fields not set via env vars are left unchanged.
//
// Recognized env vars:
//   - LOG_FORMAT: output format ("text" or "json")
//   - LOG_LEVEL: minimum log level ("debug", "info", "warn", "error")
//   - LOG_TIME_FORMAT: timestamp format ("rfc3339", "rfc3339nano", "unix", "unixmilli")
//   - LOG_ADD_SOURCE: include source file:line in output ("yes"/"no")
//   - LOG_TIME_KEY, LOG_LEVEL_KEY, LOG_MESSAGE_KEY, LOG_SOURCE_KEY: custom JSON key names
func applyLogEnvOverrides(lc *LogConfig) error {
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		lc.Format = strings.ToLower(v)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		lc.Level = strings.ToLower(v)
	}
	if v := os.Getenv("LOG_TIME_FORMAT"); v != "" {
		lc.TimeFormat = strings.ToLower(v)
	}
	if v := os.Getenv("LOG_ADD_SOURCE"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("invalid LOG_ADD_SOURCE: %w", err)
		}
		lc.AddSource = b
	}
	// Custom JSON key names (only affect JSON format output).
	if v := os.Getenv("LOG_TIME_KEY"); v != "" {
		lc.TimeKey = v
	}
	if v := os.Getenv("LOG_LEVEL_KEY"); v != "" {
		lc.LevelKey = v
	}
	if v := os.Getenv("LOG_MESSAGE_KEY"); v != "" {
		lc.MessageKey = v
	}
	if v := os.Getenv("LOG_SOURCE_KEY"); v != "" {
		lc.SourceKey = v
	}
	return nil
}

// applyDefaults fills in default values for fields not set by YAML or env vars.
// This is called after all overrides are applied, so it only fills truly unset fields.
func applyDefaults(cfg *Config) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.TimeFormat == "" {
		cfg.Log.TimeFormat = "rfc3339nano"
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 10 * time.Second
	}
	if cfg.FetchTimeout == 0 {
		cfg.FetchTimeout = 5 * time.Second
	}
	if cfg.Auth.Method == "" {
		cfg.Auth.Method = "none"
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}
	// Circuit breaker defaults.
	if cfg.CircuitBreaker != nil {
		if cfg.CircuitBreaker.MaxFailures == 0 {
			cfg.CircuitBreaker.MaxFailures = 5
		}
		if cfg.CircuitBreaker.Timeout == 0 {
			cfg.CircuitBreaker.Timeout = 60 * time.Second
		}
		if cfg.CircuitBreaker.HalfOpenLimit == 0 {
			cfg.CircuitBreaker.HalfOpenLimit = 3
		}
	}
	// HTTP transport defaults.
	if cfg.HTTPTransport != nil {
		if cfg.HTTPTransport.MaxIdleConns == 0 {
			cfg.HTTPTransport.MaxIdleConns = 100
		}
		if cfg.HTTPTransport.MaxIdleConnsPerHost == 0 {
			cfg.HTTPTransport.MaxIdleConnsPerHost = 10
		}
		if cfg.HTTPTransport.IdleConnTimeout == 0 {
			cfg.HTTPTransport.IdleConnTimeout = 90 * time.Second
		}
	}
}

// validate checks the Config for correctness after all loading and defaults.
// It ensures required fields are present, log config values are valid,
// server auth is consistent, and each dependency's auth is valid.
func validate(cfg *Config) error {
	if cfg.Name == "" {
		return fmt.Errorf("DEPHEALTH_NAME is required")
	}

	if cfg.Group == "" {
		return fmt.Errorf("DEPHEALTH_GROUP is required")
	}

	if err := validateLogConfig(&cfg.Log); err != nil {
		return err
	}

	if cfg.FetchTimeout < 0 {
		return fmt.Errorf("DEPHEALTH_FETCH_TIMEOUT must be non-negative, got %v", cfg.FetchTimeout)
	}

	if cfg.ShutdownTimeout < 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be non-negative, got %v", cfg.ShutdownTimeout)
	}

	if cfg.CircuitBreaker != nil {
		if cfg.CircuitBreaker.MaxFailures < 1 {
			return fmt.Errorf("CIRCUIT_BREAKER_MAX_FAILURES must be at least 1, got %d", cfg.CircuitBreaker.MaxFailures)
		}
		if cfg.CircuitBreaker.Timeout < 0 {
			return fmt.Errorf("CIRCUIT_BREAKER_TIMEOUT must be non-negative, got %v", cfg.CircuitBreaker.Timeout)
		}
		if cfg.CircuitBreaker.HalfOpenLimit < 1 {
			return fmt.Errorf("CIRCUIT_BREAKER_HALF_OPEN_LIMIT must be at least 1, got %d", cfg.CircuitBreaker.HalfOpenLimit)
		}
	}

	if cfg.HTTPTransport != nil {
		if cfg.HTTPTransport.MaxIdleConns < 0 {
			return fmt.Errorf("HTTP_TRANSPORT_MAX_IDLE_CONNS must be non-negative, got %d", cfg.HTTPTransport.MaxIdleConns)
		}
		if cfg.HTTPTransport.MaxIdleConnsPerHost < 0 {
			return fmt.Errorf("HTTP_TRANSPORT_MAX_IDLE_CONNS_PER_HOST must be non-negative, got %d", cfg.HTTPTransport.MaxIdleConnsPerHost)
		}
		if cfg.HTTPTransport.IdleConnTimeout < 0 {
			return fmt.Errorf("HTTP_TRANSPORT_IDLE_CONN_TIMEOUT must be non-negative, got %v", cfg.HTTPTransport.IdleConnTimeout)
		}
	}

	// Validate server-side auth for both endpoint zones.
	if err := validateServerAuth(&cfg.Auth); err != nil {
		return err
	}

	// Validate each dependency's auth config for consistency.
	for i := range cfg.Dependencies {
		if err := validateAuth(&cfg.Dependencies[i]); err != nil {
			return err
		}
	}

	return nil
}

// validateLogConfig checks that log configuration values are within the set of
// supported options. Returns an error with clear guidance on valid values.
func validateLogConfig(lc *LogConfig) error {
	switch lc.Format {
	case "text", "json":
	default:
		return fmt.Errorf("invalid LOG_FORMAT %q (expected text/json)", lc.Format)
	}

	// Use slog's own level parser to validate the level string.
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(lc.Level))); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL %q (expected debug/info/warn/error)", lc.Level)
	}

	switch lc.TimeFormat {
	case "rfc3339", "rfc3339nano", "unix", "unixmilli":
	default:
		return fmt.Errorf("invalid LOG_TIME_FORMAT %q (expected rfc3339/rfc3339nano/unix/unixmilli)", lc.TimeFormat)
	}

	return nil
}

// globalAuth holds default authentication values parsed from global env vars.
// These values serve as fallbacks for dependencies that don't define their own auth.
type globalAuth struct {
	BearerToken string
	BasicUser   string
	BasicPass   string
	Headers     map[string]string // HTTP-only custom headers
	Metadata    map[string]string // gRPC-only custom metadata
}

// loadGlobalAuth parses global authentication env vars (DEPHEALTH_BEARER_TOKEN, etc.)
// that serve as defaults for all dependencies. Per-dependency auth settings
// override these values. Supports _FILE suffix for secret values.
func loadGlobalAuth() (globalAuth, error) {
	var ga globalAuth
	var err error

	// Bearer token supports _FILE suffix for reading from mounted secret files.
	ga.BearerToken, err = resolveSecret("DEPHEALTH_BEARER_TOKEN")
	if err != nil {
		return ga, err
	}

	ga.BasicUser = os.Getenv("DEPHEALTH_BASIC_USER")

	// Basic auth password supports _FILE suffix.
	ga.BasicPass, err = resolveSecret("DEPHEALTH_BASIC_PASS")
	if err != nil {
		return ga, err
	}

	// Custom HTTP headers as JSON object: {"X-Custom": "value"}.
	if v := os.Getenv("DEPHEALTH_HEADERS"); v != "" {
		if err := json.Unmarshal([]byte(v), &ga.Headers); err != nil {
			return ga, fmt.Errorf("invalid DEPHEALTH_HEADERS JSON: %w", err)
		}
	}

	// Custom gRPC metadata as JSON object: {"x-custom": "value"}.
	if v := os.Getenv("DEPHEALTH_METADATA"); v != "" {
		if err := json.Unmarshal([]byte(v), &ga.Metadata); err != nil {
			return ga, fmt.Errorf("invalid DEPHEALTH_METADATA JSON: %w", err)
		}
	}

	return ga, nil
}

// loadServerAuth reads server-side auth from env vars (AUTH_METHOD, AUTH_USER,
// AUTH_PASS, AUTH_TOKEN, AUTH_API_KEY) and applies them to cfg.Auth.
// Also loads per-zone overrides (AUTH_STATUS_*, AUTH_METRICS_*).
// Secret values support _FILE suffix pattern.
func loadServerAuth(cfg *Config) error {
	// Global auth settings for all protected endpoints.
	if v := os.Getenv("AUTH_METHOD"); v != "" {
		cfg.Auth.Method = v
	}
	if v := os.Getenv("AUTH_USER"); v != "" {
		cfg.Auth.Username = v
	}

	// Password, token, and API key support _FILE suffix for Kubernetes secrets.
	pass, err := resolveSecret("AUTH_PASS")
	if err != nil {
		return err
	}
	if pass != "" {
		cfg.Auth.Password = pass
	}

	token, err := resolveSecret("AUTH_TOKEN")
	if err != nil {
		return err
	}
	if token != "" {
		cfg.Auth.Token = token
	}

	apiKey, err := resolveSecret("AUTH_API_KEY")
	if err != nil {
		return err
	}
	if apiKey != "" {
		cfg.Auth.APIKey = apiKey
	}

	// Per-zone auth overrides: allows different auth methods for status and metrics endpoints.
	if err := loadZoneAuth("AUTH_STATUS", &cfg.Auth, "status"); err != nil {
		return err
	}
	if err := loadZoneAuth("AUTH_METRICS", &cfg.Auth, "metrics"); err != nil {
		return err
	}

	return nil
}

// loadZoneAuth reads per-zone auth env vars (e.g. AUTH_STATUS_METHOD, AUTH_STATUS_USER)
// and creates or merges the zone override in AuthConfig.
// A zone override is only created if at least one env var for that zone is set.
// When merging with existing YAML-loaded zone config, env vars take priority.
func loadZoneAuth(prefix string, ac *AuthConfig, zone string) error {
	method := os.Getenv(prefix + "_METHOD")
	user := os.Getenv(prefix + "_USER")

	pass, err := resolveSecret(prefix + "_PASS")
	if err != nil {
		return err
	}

	token, err := resolveSecret(prefix + "_TOKEN")
	if err != nil {
		return err
	}

	apiKey, err := resolveSecret(prefix + "_API_KEY")
	if err != nil {
		return err
	}

	// Only create zone override if at least one env var is set.
	if method == "" && user == "" && pass == "" && token == "" && apiKey == "" {
		return nil
	}

	za := &ZoneAuth{
		Method:   method,
		Username: user,
		Password: pass,
		Token:    token,
		APIKey:   apiKey,
	}

	// Resolve target zone pointer in the AuthConfig.
	var target **ZoneAuth
	switch zone {
	case "status":
		target = &ac.Status
	case "metrics":
		target = &ac.Metrics
	}

	if *target == nil {
		// No existing zone config — use the new one directly.
		*target = za
	} else {
		// Merge: env values override existing YAML values (non-empty fields win).
		if za.Method != "" {
			(*target).Method = za.Method
		}
		if za.Username != "" {
			(*target).Username = za.Username
		}
		if za.Password != "" {
			(*target).Password = za.Password
		}
		if za.Token != "" {
			(*target).Token = za.Token
		}
		if za.APIKey != "" {
			(*target).APIKey = za.APIKey
		}
	}

	return nil
}

// resolveSecret resolves a secret value from an environment variable or its _FILE variant.
// This supports the common Kubernetes pattern where secrets are mounted as files.
//
// Rules:
//   - If both KEY and KEY_FILE are set, returns an error (ambiguous).
//   - If KEY_FILE is set, reads the file and returns its content (whitespace-trimmed).
//   - If only KEY is set, returns its value directly.
//   - If neither is set, returns an empty string.
func resolveSecret(envKey string) (string, error) {
	val := os.Getenv(envKey)
	fileVal := os.Getenv(envKey + "_FILE")

	// Ambiguous: both direct value and file path are set.
	if val != "" && fileVal != "" {
		return "", fmt.Errorf("both %s and %s_FILE are set; use only one", envKey, envKey)
	}

	if fileVal != "" {
		return readSecretFile(fileVal, envKey+"_FILE")
	}

	return val, nil
}

// readSecretFile reads a secret from a file path and trims leading/trailing whitespace.
// The envKey parameter is used only for error messages to identify which env var
// pointed to this file.
func readSecretFile(path, envKey string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from env var config (_FILE suffix pattern)
	if err != nil {
		return "", fmt.Errorf("%s: cannot read file %q: %w", envKey, path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// validateAuth checks for conflicting auth methods on a single dependency.
// Only one auth method is allowed per dependency: bearer token, basic auth,
// custom headers, or custom metadata.
//
// Also validates:
//   - Incomplete basic auth (username without password or vice versa).
//   - Type-appropriate auth: headers only for HTTP, metadata only for gRPC.
func validateAuth(dep *Dependency) error {
	// Count the number of auth methods configured.
	methods := 0
	if dep.BearerToken != "" {
		methods++
	}
	if dep.BasicUser != "" || dep.BasicPass != "" {
		methods++
	}
	if len(dep.Headers) > 0 {
		methods++
	}
	if len(dep.Metadata) > 0 {
		methods++
	}

	if methods > 1 {
		return fmt.Errorf("dependency %q: conflicting auth methods; specify only one of bearer token, basic auth, headers, or metadata", dep.Name)
	}

	// Incomplete Basic Auth: both username and password must be set together.
	if dep.BasicUser != "" && dep.BasicPass == "" {
		return fmt.Errorf("dependency %q: BASIC_USER is set but BASIC_PASS is missing", dep.Name)
	}
	if dep.BasicPass != "" && dep.BasicUser == "" {
		return fmt.Errorf("dependency %q: BASIC_PASS is set but BASIC_USER is missing", dep.Name)
	}

	// Type-appropriate check: headers are HTTP-only, metadata is gRPC-only.
	if len(dep.Headers) > 0 && dep.Type != "http" {
		return fmt.Errorf("dependency %q: HEADERS is only valid for HTTP dependencies, got type %q", dep.Name, dep.Type)
	}
	if len(dep.Metadata) > 0 && dep.Type != "grpc" {
		return fmt.Errorf("dependency %q: METADATA is only valid for gRPC dependencies, got type %q", dep.Name, dep.Type)
	}

	return nil
}

// validateServerAuth validates the resolved auth config for both endpoint zones
// ("status" and "metrics"). Ensures that the chosen auth method has all required
// credentials (e.g. basic requires username+password, bearer requires token).
func validateServerAuth(ac *AuthConfig) error {
	for _, zone := range []string{"status", "metrics"} {
		za := ac.ResolveZone(zone)
		switch za.Method {
		case "none":
			// OK — no auth required for this zone.
		case "basic":
			if za.Username == "" || za.Password == "" {
				return fmt.Errorf("auth %s: method=basic requires username and password", zone)
			}
		case "bearer":
			if za.Token == "" {
				return fmt.Errorf("auth %s: method=bearer requires token", zone)
			}
		case "apikey":
			if za.APIKey == "" {
				return fmt.Errorf("auth %s: method=apikey requires api_key", zone)
			}
		default:
			return fmt.Errorf("auth %s: unsupported method %q", zone, za.Method)
		}
	}
	return nil
}

// parseDeps parses the DEPHEALTH_DEPS string ("name1:type1,name2:type2,...") into
// a slice of fully configured Dependency structs. For each dependency, it reads
// per-dependency env vars (DEPHEALTH_<NAME>_URL, etc.) and applies global auth defaults.
//
// Supported types: http, redis, postgres, grpc, tcp, mysql, amqp, kafka, ldap.
func parseDeps(s string, ga globalAuth) ([]Dependency, error) {
	pairs := strings.Split(s, ",")
	deps := make([]Dependency, 0, len(pairs))

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		// Each pair must be "name:type".
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid dependency format %q, expected name:type", pair)
		}
		name := parts[0]
		depType := parts[1]

		// Validate dependency type against the supported set.
		switch depType {
		case "http", "redis", "postgres", "grpc", "tcp", "mysql", "amqp", "kafka", "ldap":
		default:
			return nil, fmt.Errorf("dependency %q: unsupported type %q", name, depType)
		}

		dep, err := parseSingleDep(name, depType, ga)
		if err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}

	return deps, nil
}

// parseSingleDep reads per-dependency env vars for a given dependency name and type.
// It constructs the env var prefix as "DEPHEALTH_<NAME>_" where NAME is the uppercased
// dependency name with hyphens replaced by underscores (see EnvName).
//
// Required env vars per dependency:
//   - DEPHEALTH_<NAME>_URL or DEPHEALTH_<NAME>_HOST + DEPHEALTH_<NAME>_PORT
//   - DEPHEALTH_<NAME>_CRITICAL (yes/no)
//
// Optional env vars depend on the dependency type and include health paths,
// TLS settings, authentication, and type-specific configuration.
func parseSingleDep(name, depType string, ga globalAuth) (Dependency, error) {
	prefix := "DEPHEALTH_" + EnvName(name) + "_"

	dep := Dependency{
		Name: name,
		Type: depType,
	}

	// Connection source: URL takes priority over Host+Port.
	dep.URL = os.Getenv(prefix + "URL")
	if dep.URL == "" {
		dep.Host = os.Getenv(prefix + "HOST")
		dep.Port = os.Getenv(prefix + "PORT")
		if dep.Host == "" || dep.Port == "" {
			return dep, fmt.Errorf("dependency %q: either %sURL or %sHOST + %sPORT is required",
				name, prefix, prefix, prefix)
		}
	}

	// Critical flag: determines if this dependency affects application readiness.
	critStr := os.Getenv(prefix + "CRITICAL")
	switch strings.ToLower(critStr) {
	case "yes", "true", "1":
		dep.Critical = true
	case "no", "false", "0":
		dep.Critical = false
	case "":
		return dep, fmt.Errorf("dependency %q: %sCRITICAL is required (yes/no)", name, prefix)
	default:
		return dep, fmt.Errorf("dependency %q: invalid %sCRITICAL value %q (expected yes/no)",
			name, prefix, critStr)
	}

	// Per-dependency check interval override (in seconds).
	if v := os.Getenv(prefix + "CHECK_INTERVAL"); v != "" {
		s, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return dep, fmt.Errorf("dependency %q: invalid %sCHECK_INTERVAL %q: %w", name, prefix, v, err)
		}
		dep.CheckInterval = time.Duration(s * float64(time.Second))
	}

	// Per-dependency timeout override (in seconds).
	if v := os.Getenv(prefix + "TIMEOUT"); v != "" {
		s, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return dep, fmt.Errorf("dependency %q: invalid %sTIMEOUT %q: %w", name, prefix, v, err)
		}
		dep.Timeout = time.Duration(s * float64(time.Second))
	}

	// Type-specific options: each dependency type has its own set of optional env vars.
	switch depType {
	case "http":
		dep.HealthPath = os.Getenv(prefix + "HEALTH_PATH")
		if err := parseTLSOptions(&dep, name, prefix); err != nil {
			return dep, err
		}
		dep.HostHeader = os.Getenv(prefix + "HOST_HEADER")

	case "grpc":
		dep.GRPCServiceName = os.Getenv(prefix + "GRPC_SERVICE_NAME")
		if err := parseTLSOptions(&dep, name, prefix); err != nil {
			return dep, err
		}
		dep.GRPCAuthority = os.Getenv(prefix + "GRPC_AUTHORITY")

	case "postgres":
		dep.PostgresQuery = os.Getenv(prefix + "POSTGRES_QUERY")

	case "mysql":
		dep.MySQLQuery = os.Getenv(prefix + "MYSQL_QUERY")

	case "redis":
		dep.RedisPassword = os.Getenv(prefix + "REDIS_PASSWORD")
		if v := os.Getenv(prefix + "REDIS_DB"); v != "" {
			db, err := strconv.Atoi(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sREDIS_DB %q: %w", name, prefix, v, err)
			}
			dep.RedisDB = &db
		}

	case "amqp":
		dep.AMQPURL = os.Getenv(prefix + "AMQP_URL")

	case "ldap":
		// LDAP check method with default "root_dse" (simplest, no credentials needed).
		dep.LDAPCheckMethod = os.Getenv(prefix + "LDAP_CHECK_METHOD")
		if dep.LDAPCheckMethod == "" {
			dep.LDAPCheckMethod = "root_dse"
		}
		// Validate against supported check methods.
		switch dep.LDAPCheckMethod {
		case "anonymous_bind", "simple_bind", "root_dse", "search":
		default:
			return dep, fmt.Errorf("dependency %q: invalid %sLDAP_CHECK_METHOD %q (expected anonymous_bind/simple_bind/root_dse/search)",
				name, prefix, dep.LDAPCheckMethod)
		}

		// Bind credentials for simple_bind method.
		dep.LDAPBindDN = os.Getenv(prefix + "LDAP_BIND_DN")

		var ldapErr error
		dep.LDAPBindPassword, ldapErr = resolveSecret(prefix + "LDAP_BIND_PASSWORD")
		if ldapErr != nil {
			return dep, fmt.Errorf("dependency %q: %w", name, ldapErr)
		}

		// Search parameters for search check method.
		dep.LDAPBaseDN = os.Getenv(prefix + "LDAP_BASE_DN")
		dep.LDAPSearchFilter = os.Getenv(prefix + "LDAP_SEARCH_FILTER")

		dep.LDAPSearchScope = os.Getenv(prefix + "LDAP_SEARCH_SCOPE")
		if dep.LDAPSearchScope != "" {
			switch dep.LDAPSearchScope {
			case "base", "one", "sub":
			default:
				return dep, fmt.Errorf("dependency %q: invalid %sLDAP_SEARCH_SCOPE %q (expected base/one/sub)",
					name, prefix, dep.LDAPSearchScope)
			}
		}

		// TLS options for LDAP connections.
		if v := os.Getenv(prefix + "LDAP_START_TLS"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sLDAP_START_TLS: %w", name, prefix, err)
			}
			dep.LDAPStartTLS = &b
		}
		if v := os.Getenv(prefix + "LDAP_TLS_SKIP_VERIFY"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sLDAP_TLS_SKIP_VERIFY: %w", name, prefix, err)
			}
			dep.LDAPTLSSkipVerify = &b
		}
	}

	// Authentication: resolve per-dep values, fall back to global defaults.
	if err := loadDepAuth(&dep, prefix, ga); err != nil {
		return dep, err
	}

	// Validate auth consistency (no conflicting methods, complete basic auth, etc.).
	if err := validateAuth(&dep); err != nil {
		return dep, err
	}

	return dep, nil
}

// loadDepAuth parses auth env vars for a single dependency with global fallback.
// Per-dependency auth settings always override global defaults.
//
// Resolution logic:
//  1. Read per-dep values (DEPHEALTH_<NAME>_BEARER_TOKEN, BASIC_USER, BASIC_PASS).
//  2. If per-dep value is set, use it; otherwise fall back to global default.
//  3. If per-dep sets one auth method, clear any inherited values from a different method.
//  4. Custom headers (HTTP) and metadata (gRPC) follow the same per-dep > global pattern.
func loadDepAuth(dep *Dependency, prefix string, ga globalAuth) error {
	// Bearer token: per-dep > global.
	perDepToken, err := resolveSecret(prefix + "BEARER_TOKEN")
	if err != nil {
		return fmt.Errorf("dependency %q: %w", dep.Name, err)
	}
	if perDepToken != "" {
		dep.BearerToken = perDepToken
	} else if ga.BearerToken != "" {
		dep.BearerToken = ga.BearerToken
	}

	// Basic Auth username: per-dep > global.
	perDepUser := os.Getenv(prefix + "BASIC_USER")
	if perDepUser != "" {
		dep.BasicUser = perDepUser
	} else if ga.BasicUser != "" {
		dep.BasicUser = ga.BasicUser
	}

	// Basic Auth password: per-dep > global.
	perDepPass, err := resolveSecret(prefix + "BASIC_PASS")
	if err != nil {
		return fmt.Errorf("dependency %q: %w", dep.Name, err)
	}
	if perDepPass != "" {
		dep.BasicPass = perDepPass
	} else if ga.BasicPass != "" {
		dep.BasicPass = ga.BasicPass
	}

	// Conflict check: per-dep cannot set both bearer and basic auth.
	hasPerDepBearer := perDepToken != ""
	hasPerDepBasic := perDepUser != "" || perDepPass != ""
	if hasPerDepBearer && hasPerDepBasic {
		return fmt.Errorf("dependency %q: both BEARER_TOKEN and BASIC_USER/BASIC_PASS are set; use only one auth method", dep.Name)
	}

	// Clean up: if per-dep explicitly sets one method, clear inherited values from the other.
	// This prevents conflicts when global sets bearer but per-dep sets basic (or vice versa).
	if hasPerDepBearer {
		dep.BasicUser = ""
		dep.BasicPass = ""
	} else if hasPerDepBasic {
		dep.BearerToken = ""
	}

	// Custom HTTP headers: per-dep JSON completely replaces global headers.
	if v := os.Getenv(prefix + "HEADERS"); v != "" {
		dep.Headers = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &dep.Headers); err != nil {
			return fmt.Errorf("dependency %q: invalid %sHEADERS JSON: %w", dep.Name, prefix, err)
		}
	} else if dep.Type == "http" && len(ga.Headers) > 0 {
		// Inherit global headers only for HTTP dependencies.
		dep.Headers = ga.Headers
	}

	// Custom gRPC metadata: per-dep JSON completely replaces global metadata.
	if v := os.Getenv(prefix + "METADATA"); v != "" {
		dep.Metadata = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &dep.Metadata); err != nil {
			return fmt.Errorf("dependency %q: invalid %sMETADATA JSON: %w", dep.Name, prefix, err)
		}
	} else if dep.Type == "grpc" && len(ga.Metadata) > 0 {
		// Inherit global metadata only for gRPC dependencies.
		dep.Metadata = ga.Metadata
	}

	return nil
}

// EnvName converts a dependency name to environment variable format by uppercasing
// and replacing hyphens with underscores.
// Example: "uniproxy-02" -> "UNIPROXY_02", "my-redis" -> "MY_REDIS".
func EnvName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// parseTLSOptions parses TLS and TLS_SKIP_VERIFY boolean env vars for HTTP and gRPC
// dependencies. Uses pointer types (*bool) so nil indicates "not set" (use checker default).
func parseTLSOptions(dep *Dependency, name, prefix string) error {
	if v := os.Getenv(prefix + "TLS"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("dependency %q: invalid %sTLS: %w", name, prefix, err)
		}
		dep.TLS = &b
	}
	if v := os.Getenv(prefix + "TLS_SKIP_VERIFY"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("dependency %q: invalid %sTLS_SKIP_VERIFY: %w", name, prefix, err)
		}
		dep.TLSSkipVerify = &b
	}
	return nil
}

// parseBool parses common boolean string representations.
// Accepts: "yes"/"no", "true"/"false", "1"/"0" (case-insensitive).
// Returns an error for any unrecognized value.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "yes", "true", "1":
		return true, nil
	case "no", "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q (expected yes/no/true/false/1/0)", s)
	}
}
