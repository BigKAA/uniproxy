// Package config provides environment variable configuration parsing for uniproxy.
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

// Config is the top-level application configuration.
type Config struct {
	Name          string
	Group         string // logical group for metrics label
	ListenAddr    string
	Log           LogConfig
	Auth          AuthConfig
	CheckInterval time.Duration
	Timeout       time.Duration // global check timeout (0 = SDK default)
	FetchTimeout  time.Duration // timeout for recursive HTTP fetch (default 5s)
	IsEntry       bool          // when true, isentry=yes label is added to all dependency metrics
	Dependencies  []Dependency
}

// AuthConfig describes server-side (incoming) authentication.
type AuthConfig struct {
	// Global defaults (applied to all protected zones).
	Method   string // "none" | "basic" | "bearer" | "apikey"
	Username string
	Password string
	Token    string
	APIKey   string

	// Per-zone overrides.
	Status  *ZoneAuth // override for /
	Metrics *ZoneAuth // override for /metrics
}

// ZoneAuth is a per-zone auth override.
type ZoneAuth struct {
	Method   string
	Username string
	Password string
	Token    string
	APIKey   string
}

// ResolveZone returns the effective auth configuration for a zone.
// Zone-specific values take priority, then global, then "none".
func (c *AuthConfig) ResolveZone(zone string) ZoneAuth {
	var za *ZoneAuth
	switch zone {
	case "status":
		za = c.Status
	case "metrics":
		za = c.Metrics
	}

	result := ZoneAuth{
		Method:   c.Method,
		Username: c.Username,
		Password: c.Password,
		Token:    c.Token,
		APIKey:   c.APIKey,
	}

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

	if result.Method == "" {
		result.Method = "none"
	}
	return result
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

	// Authentication (mutually exclusive: only one method per dependency).
	BearerToken string
	BasicUser   string
	BasicPass   string
	Headers     map[string]string // HTTP-only: custom headers
	Metadata    map[string]string // gRPC-only: custom metadata
}

// Load parses configuration from YAML file (optional) and environment variables.
// Environment variables always take priority over YAML values.
//
// Flow: CONFIG_FILE → loadFromYAML → applyEnvOverrides → applyDefaults → validate.
func Load() (*Config, error) {
	var cfg *Config

	// Phase 1: Base config from YAML (if provided).
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

	// Phase 4: Validate.
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides reads environment variables and overwrites the corresponding
// Config fields. Fields not set via env vars are left unchanged.
func applyEnvOverrides(cfg *Config) error {
	// Listen address.
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	// Log config overlay.
	if err := applyLogEnvOverrides(&cfg.Log); err != nil {
		return err
	}

	// Application name.
	if v := os.Getenv("DEPHEALTH_NAME"); v != "" {
		cfg.Name = v
	}

	// Application group.
	if v := os.Getenv("DEPHEALTH_GROUP"); v != "" {
		cfg.Group = v
	}

	// Check interval.
	if v := os.Getenv("DEPHEALTH_CHECK_INTERVAL"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_CHECK_INTERVAL %q: %w", v, err)
		}
		cfg.CheckInterval = time.Duration(sec * float64(time.Second))
	}

	// Global timeout (optional).
	if v := os.Getenv("DEPHEALTH_TIMEOUT"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_TIMEOUT %q: %w", v, err)
		}
		cfg.Timeout = time.Duration(sec * float64(time.Second))
	}

	// Fetch timeout.
	if v := os.Getenv("DEPHEALTH_FETCH_TIMEOUT"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_FETCH_TIMEOUT %q: %w", v, err)
		}
		cfg.FetchTimeout = time.Duration(sec * float64(time.Second))
	}

	// IsEntry flag (optional global label).
	if v := os.Getenv("DEPHEALTH_ISENTRY"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("invalid DEPHEALTH_ISENTRY: %w", err)
		}
		cfg.IsEntry = b
	}

	// Server-side (incoming) auth.
	if err := loadServerAuth(cfg); err != nil {
		return err
	}

	// Global auth defaults for dependencies.
	ga, err := loadGlobalAuth()
	if err != nil {
		return err
	}

	// Dependencies: if DEPHEALTH_DEPS is set, it REPLACES any YAML deps entirely.
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
}

// validate checks the Config for correctness after all loading and defaults.
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

	// Validate server-side auth.
	if err := validateServerAuth(&cfg.Auth); err != nil {
		return err
	}

	// Validate dependency auth.
	for i := range cfg.Dependencies {
		if err := validateAuth(&cfg.Dependencies[i]); err != nil {
			return err
		}
	}

	return nil
}

// validateLogConfig checks that log configuration values are valid.
func validateLogConfig(lc *LogConfig) error {
	switch lc.Format {
	case "text", "json":
	default:
		return fmt.Errorf("invalid LOG_FORMAT %q (expected text/json)", lc.Format)
	}

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
type globalAuth struct {
	BearerToken string
	BasicUser   string
	BasicPass   string
	Headers     map[string]string
	Metadata    map[string]string
}

// loadGlobalAuth parses global authentication env vars (DEPHEALTH_BEARER_TOKEN, etc.).
func loadGlobalAuth() (globalAuth, error) {
	var ga globalAuth
	var err error

	ga.BearerToken, err = resolveSecret("DEPHEALTH_BEARER_TOKEN")
	if err != nil {
		return ga, err
	}

	ga.BasicUser = os.Getenv("DEPHEALTH_BASIC_USER")

	ga.BasicPass, err = resolveSecret("DEPHEALTH_BASIC_PASS")
	if err != nil {
		return ga, err
	}

	if v := os.Getenv("DEPHEALTH_HEADERS"); v != "" {
		if err := json.Unmarshal([]byte(v), &ga.Headers); err != nil {
			return ga, fmt.Errorf("invalid DEPHEALTH_HEADERS JSON: %w", err)
		}
	}

	if v := os.Getenv("DEPHEALTH_METADATA"); v != "" {
		if err := json.Unmarshal([]byte(v), &ga.Metadata); err != nil {
			return ga, fmt.Errorf("invalid DEPHEALTH_METADATA JSON: %w", err)
		}
	}

	return ga, nil
}

// loadServerAuth reads server-side auth from env vars and applies to cfg.Auth.
func loadServerAuth(cfg *Config) error {
	// Global auth settings.
	if v := os.Getenv("AUTH_METHOD"); v != "" {
		cfg.Auth.Method = v
	}
	if v := os.Getenv("AUTH_USER"); v != "" {
		cfg.Auth.Username = v
	}

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

	// Per-zone: status.
	if err := loadZoneAuth("AUTH_STATUS", &cfg.Auth, "status"); err != nil {
		return err
	}
	// Per-zone: metrics.
	if err := loadZoneAuth("AUTH_METRICS", &cfg.Auth, "metrics"); err != nil {
		return err
	}

	return nil
}

// loadZoneAuth reads per-zone auth env vars and sets the zone override.
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

	// Resolve target zone pointer.
	var target **ZoneAuth
	switch zone {
	case "status":
		target = &ac.Status
	case "metrics":
		target = &ac.Metrics
	}

	if *target == nil {
		*target = za
	} else {
		// Merge: env values override existing YAML values.
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

// resolveSecret resolves a secret value from env var or _FILE variant.
// If both KEY and KEY_FILE are set, returns an error.
// If KEY_FILE is set, reads the file and returns its trimmed content.
func resolveSecret(envKey string) (string, error) {
	val := os.Getenv(envKey)
	fileVal := os.Getenv(envKey + "_FILE")

	if val != "" && fileVal != "" {
		return "", fmt.Errorf("both %s and %s_FILE are set; use only one", envKey, envKey)
	}

	if fileVal != "" {
		return readSecretFile(fileVal, envKey+"_FILE")
	}

	return val, nil
}

// readSecretFile reads a secret from a file path, trimming whitespace.
func readSecretFile(path, envKey string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: cannot read file %q: %w", envKey, path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// validateAuth checks for conflicting auth methods on a dependency.
func validateAuth(dep *Dependency) error {
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

	// Incomplete Basic Auth.
	if dep.BasicUser != "" && dep.BasicPass == "" {
		return fmt.Errorf("dependency %q: BASIC_USER is set but BASIC_PASS is missing", dep.Name)
	}
	if dep.BasicPass != "" && dep.BasicUser == "" {
		return fmt.Errorf("dependency %q: BASIC_PASS is set but BASIC_USER is missing", dep.Name)
	}

	// Type-appropriate check.
	if len(dep.Headers) > 0 && dep.Type != "http" {
		return fmt.Errorf("dependency %q: HEADERS is only valid for HTTP dependencies, got type %q", dep.Name, dep.Type)
	}
	if len(dep.Metadata) > 0 && dep.Type != "grpc" {
		return fmt.Errorf("dependency %q: METADATA is only valid for gRPC dependencies, got type %q", dep.Name, dep.Type)
	}

	return nil
}

// validateServerAuth validates the resolved auth config for both zones.
func validateServerAuth(ac *AuthConfig) error {
	for _, zone := range []string{"status", "metrics"} {
		za := ac.ResolveZone(zone)
		switch za.Method {
		case "none":
			// OK — no auth required.
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

// parseDeps parses "name1:type1,name2:type2,..." into a slice of Dependency.
func parseDeps(s string, ga globalAuth) ([]Dependency, error) {
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

		dep, err := parseSingleDep(name, depType, ga)
		if err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}

	return deps, nil
}

// parseSingleDep reads per-dependency env vars for a given dependency.
func parseSingleDep(name, depType string, ga globalAuth) (Dependency, error) {
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
		if err := parseTLSOptions(&dep, name, prefix); err != nil {
			return dep, err
		}

	case "grpc":
		dep.GRPCServiceName = os.Getenv(prefix + "GRPC_SERVICE_NAME")
		if err := parseTLSOptions(&dep, name, prefix); err != nil {
			return dep, err
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

	// Authentication: resolve per-dep values, fall back to global.
	if err := loadDepAuth(&dep, prefix, ga); err != nil {
		return dep, err
	}

	if err := validateAuth(&dep); err != nil {
		return dep, err
	}

	return dep, nil
}

// loadDepAuth parses auth env vars for a single dependency with global fallback.
func loadDepAuth(dep *Dependency, prefix string, ga globalAuth) error {
	// Bearer token.
	perDepToken, err := resolveSecret(prefix + "BEARER_TOKEN")
	if err != nil {
		return fmt.Errorf("dependency %q: %w", dep.Name, err)
	}
	if perDepToken != "" {
		dep.BearerToken = perDepToken
	} else if ga.BearerToken != "" {
		dep.BearerToken = ga.BearerToken
	}

	// Basic Auth username.
	perDepUser := os.Getenv(prefix + "BASIC_USER")
	if perDepUser != "" {
		dep.BasicUser = perDepUser
	} else if ga.BasicUser != "" {
		dep.BasicUser = ga.BasicUser
	}

	// Basic Auth password.
	perDepPass, err := resolveSecret(prefix + "BASIC_PASS")
	if err != nil {
		return fmt.Errorf("dependency %q: %w", dep.Name, err)
	}
	if perDepPass != "" {
		dep.BasicPass = perDepPass
	} else if ga.BasicPass != "" {
		dep.BasicPass = ga.BasicPass
	}

	// If per-dep sets both bearer and basic — that's a conflict.
	hasPerDepBearer := perDepToken != ""
	hasPerDepBasic := perDepUser != "" || perDepPass != ""
	if hasPerDepBearer && hasPerDepBasic {
		return fmt.Errorf("dependency %q: both BEARER_TOKEN and BASIC_USER/BASIC_PASS are set; use only one auth method", dep.Name)
	}

	// Override logic: if any per-dep auth is set, clear inherited global values
	// that belong to a different auth method.
	if hasPerDepBearer {
		dep.BasicUser = ""
		dep.BasicPass = ""
	} else if hasPerDepBasic {
		dep.BearerToken = ""
	}

	// Custom HTTP headers (per-dep overrides global entirely).
	if v := os.Getenv(prefix + "HEADERS"); v != "" {
		dep.Headers = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &dep.Headers); err != nil {
			return fmt.Errorf("dependency %q: invalid %sHEADERS JSON: %w", dep.Name, prefix, err)
		}
	} else if dep.Type == "http" && len(ga.Headers) > 0 {
		dep.Headers = ga.Headers
	}

	// Custom gRPC metadata (per-dep overrides global entirely).
	if v := os.Getenv(prefix + "METADATA"); v != "" {
		dep.Metadata = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &dep.Metadata); err != nil {
			return fmt.Errorf("dependency %q: invalid %sMETADATA JSON: %w", dep.Name, prefix, err)
		}
	} else if dep.Type == "grpc" && len(ga.Metadata) > 0 {
		dep.Metadata = ga.Metadata
	}

	return nil
}

// EnvName converts a dependency name to environment variable format:
// "uniproxy-02" → "UNIPROXY_02".
func EnvName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// parseTLSOptions parses TLS and TLS_SKIP_VERIFY env vars for a dependency.
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
