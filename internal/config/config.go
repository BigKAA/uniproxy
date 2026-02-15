// Package config provides environment variable configuration parsing for uniproxy.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the top-level application configuration.
type Config struct {
	Name          string
	ListenAddr    string
	Log           LogConfig
	CheckInterval time.Duration
	Timeout       time.Duration // global check timeout (0 = SDK default)
	FetchTimeout  time.Duration // timeout for recursive HTTP fetch (default 5s)
	Dependencies  []Dependency
}

// LogConfig holds logging configuration parsed from environment variables.
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

// Dependency describes a single dependency to health-check.
type Dependency struct {
	Name       string
	Type       string // http, redis, postgres, grpc, tcp, mysql, amqp, kafka
	URL        string
	Host       string
	Port       string
	Critical   bool
	HealthPath string // HTTP-specific

	// Per-dependency timing (0 = use global).
	CheckInterval time.Duration
	Timeout       time.Duration

	// TLS options (HTTP, gRPC).
	TLS           *bool
	TLSSkipVerify *bool

	// gRPC-specific.
	GRPCServiceName string

	// PostgreSQL/MySQL-specific.
	PostgresQuery string
	MySQLQuery    string

	// Redis-specific.
	RedisPassword string
	RedisDB       *int

	// AMQP-specific.
	AMQPURL string
}

// Load parses configuration from environment variables.
//
// Required:
//   - DEPHEALTH_NAME — application name
//   - DEPHEALTH_DEPS — comma-separated "name:type" pairs
//
// Optional:
//   - LISTEN_ADDR (default ":8080")
//   - DEPHEALTH_CHECK_INTERVAL — seconds (default "10")
//   - DEPHEALTH_FETCH_TIMEOUT — seconds, timeout for recursive HTTP detail fetch (default "5")
//
// Logging:
//   - LOG_FORMAT — "text" or "json" (default "text")
//   - LOG_LEVEL — "debug", "info", "warn", "error" (default "info")
//   - LOG_TIME_FORMAT — "rfc3339", "rfc3339nano", "unix", "unixmilli" (default "rfc3339nano")
//   - LOG_ADD_SOURCE — "yes"/"no" (default "no")
//   - LOG_TIME_KEY, LOG_LEVEL_KEY, LOG_MESSAGE_KEY, LOG_SOURCE_KEY — custom JSON key names
//
// Per-dependency (NAME is uppercase with hyphens replaced by underscores):
//   - DEPHEALTH_<NAME>_URL or DEPHEALTH_<NAME>_HOST + DEPHEALTH_<NAME>_PORT
//   - DEPHEALTH_<NAME>_CRITICAL — "yes" or "no"
//   - DEPHEALTH_<NAME>_HEALTH_PATH — HTTP health check path
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
	}

	// Log config.
	logCfg, err := loadLogConfig()
	if err != nil {
		return nil, err
	}
	cfg.Log = logCfg

	// Application name.
	cfg.Name = os.Getenv("DEPHEALTH_NAME")
	if cfg.Name == "" {
		return nil, fmt.Errorf("DEPHEALTH_NAME is required")
	}

	// Check interval.
	intervalStr := getEnv("DEPHEALTH_CHECK_INTERVAL", "10")
	sec, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid DEPHEALTH_CHECK_INTERVAL %q: %w", intervalStr, err)
	}
	cfg.CheckInterval = time.Duration(sec * float64(time.Second))

	// Global timeout (optional).
	if ts := os.Getenv("DEPHEALTH_TIMEOUT"); ts != "" {
		tSec, err := strconv.ParseFloat(ts, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid DEPHEALTH_TIMEOUT %q: %w", ts, err)
		}
		cfg.Timeout = time.Duration(tSec * float64(time.Second))
	}

	// Fetch timeout for recursive HTTP detail requests (default 5s).
	fetchStr := getEnv("DEPHEALTH_FETCH_TIMEOUT", "5")
	fetchSec, err := strconv.ParseFloat(fetchStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid DEPHEALTH_FETCH_TIMEOUT %q: %w", fetchStr, err)
	}
	if fetchSec < 0 {
		return nil, fmt.Errorf("DEPHEALTH_FETCH_TIMEOUT must be non-negative, got %v", fetchSec)
	}
	cfg.FetchTimeout = time.Duration(fetchSec * float64(time.Second))

	// Dependencies (optional — service may have none).
	depsStr := os.Getenv("DEPHEALTH_DEPS")
	if depsStr != "" {
		deps, err := parseDeps(depsStr)
		if err != nil {
			return nil, err
		}
		cfg.Dependencies = deps
	}

	return cfg, nil
}

// loadLogConfig parses LOG_* environment variables into LogConfig.
func loadLogConfig() (LogConfig, error) {
	lc := LogConfig{
		Format:     strings.ToLower(getEnv("LOG_FORMAT", "text")),
		Level:      strings.ToLower(getEnv("LOG_LEVEL", "info")),
		TimeFormat: strings.ToLower(getEnv("LOG_TIME_FORMAT", "rfc3339nano")),
		TimeKey:    os.Getenv("LOG_TIME_KEY"),
		LevelKey:   os.Getenv("LOG_LEVEL_KEY"),
		MessageKey: os.Getenv("LOG_MESSAGE_KEY"),
		SourceKey:  os.Getenv("LOG_SOURCE_KEY"),
	}

	// Validate format.
	switch lc.Format {
	case "text", "json":
	default:
		return lc, fmt.Errorf("invalid LOG_FORMAT %q (expected text/json)", lc.Format)
	}

	// Validate level.
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(lc.Level))); err != nil {
		return lc, fmt.Errorf("invalid LOG_LEVEL %q (expected debug/info/warn/error)", lc.Level)
	}

	// Validate time format.
	switch lc.TimeFormat {
	case "rfc3339", "rfc3339nano", "unix", "unixmilli":
	default:
		return lc, fmt.Errorf("invalid LOG_TIME_FORMAT %q (expected rfc3339/rfc3339nano/unix/unixmilli)", lc.TimeFormat)
	}

	// AddSource (optional).
	if v := os.Getenv("LOG_ADD_SOURCE"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return lc, fmt.Errorf("invalid LOG_ADD_SOURCE: %w", err)
		}
		lc.AddSource = b
	}

	return lc, nil
}

// parseDeps parses "name1:type1,name2:type2,..." into a slice of Dependency.
func parseDeps(s string) ([]Dependency, error) {
	pairs := strings.Split(s, ",")
	deps := make([]Dependency, 0, len(pairs))

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid dependency format %q, expected name:type", pair)
		}
		name := parts[0]
		depType := parts[1]

		switch depType {
		case "http", "redis", "postgres", "grpc", "tcp", "mysql", "amqp", "kafka":
		default:
			return nil, fmt.Errorf("dependency %q: unsupported type %q", name, depType)
		}

		dep, err := parseSingleDep(name, depType)
		if err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}

	return deps, nil
}

// parseSingleDep reads per-dependency env vars for a given dependency.
func parseSingleDep(name, depType string) (Dependency, error) {
	prefix := "DEPHEALTH_" + EnvName(name) + "_"

	dep := Dependency{
		Name: name,
		Type: depType,
	}

	// URL or Host+Port.
	dep.URL = os.Getenv(prefix + "URL")
	if dep.URL == "" {
		dep.Host = os.Getenv(prefix + "HOST")
		dep.Port = os.Getenv(prefix + "PORT")
		if dep.Host == "" || dep.Port == "" {
			return dep, fmt.Errorf("dependency %q: either %sURL or %sHOST + %sPORT is required",
				name, prefix, prefix, prefix)
		}
	}

	// Critical flag.
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

	// Per-dependency check interval.
	if v := os.Getenv(prefix + "CHECK_INTERVAL"); v != "" {
		s, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return dep, fmt.Errorf("dependency %q: invalid %sCHECK_INTERVAL %q: %w", name, prefix, v, err)
		}
		dep.CheckInterval = time.Duration(s * float64(time.Second))
	}

	// Per-dependency timeout.
	if v := os.Getenv(prefix + "TIMEOUT"); v != "" {
		s, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return dep, fmt.Errorf("dependency %q: invalid %sTIMEOUT %q: %w", name, prefix, v, err)
		}
		dep.Timeout = time.Duration(s * float64(time.Second))
	}

	// Type-specific options.
	switch depType {
	case "http":
		dep.HealthPath = os.Getenv(prefix + "HEALTH_PATH")
		if v := os.Getenv(prefix + "TLS"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sTLS: %w", name, prefix, err)
			}
			dep.TLS = &b
		}
		if v := os.Getenv(prefix + "TLS_SKIP_VERIFY"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sTLS_SKIP_VERIFY: %w", name, prefix, err)
			}
			dep.TLSSkipVerify = &b
		}

	case "grpc":
		dep.GRPCServiceName = os.Getenv(prefix + "GRPC_SERVICE_NAME")
		if v := os.Getenv(prefix + "TLS"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sTLS: %w", name, prefix, err)
			}
			dep.TLS = &b
		}
		if v := os.Getenv(prefix + "TLS_SKIP_VERIFY"); v != "" {
			b, err := parseBool(v)
			if err != nil {
				return dep, fmt.Errorf("dependency %q: invalid %sTLS_SKIP_VERIFY: %w", name, prefix, err)
			}
			dep.TLSSkipVerify = &b
		}

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
	}

	return dep, nil
}

// EnvName converts a dependency name to environment variable format:
// "uniproxy-02" → "UNIPROXY_02".
func EnvName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseBool parses common boolean strings.
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
