package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// yamlConfig is the intermediate YAML representation.
// Duration fields use string to support both Go duration ("15s") and plain seconds ("15").
type yamlConfig struct {
	Name          string         `yaml:"name"`
	Group         string         `yaml:"group"`
	ListenAddr    string         `yaml:"listenAddr"`
	Log           yamlLogConfig  `yaml:"log"`
	Auth          yamlAuthConfig `yaml:"auth"`
	CheckInterval string         `yaml:"checkInterval"`
	Timeout       string         `yaml:"timeout"`
	FetchTimeout  string         `yaml:"fetchTimeout"`
	IsEntry       *bool          `yaml:"isEntry"`
	Dependencies  []yamlDep      `yaml:"dependencies"`
}

// yamlLogConfig mirrors LogConfig with YAML tags.
type yamlLogConfig struct {
	Format     string `yaml:"format"`
	Level      string `yaml:"level"`
	TimeFormat string `yaml:"timeFormat"`
	AddSource  *bool  `yaml:"addSource"`
	TimeKey    string `yaml:"timeKey"`
	LevelKey   string `yaml:"levelKey"`
	MessageKey string `yaml:"messageKey"`
	SourceKey  string `yaml:"sourceKey"`
}

// yamlAuthConfig describes server-side (incoming) authentication in YAML.
type yamlAuthConfig struct {
	Method   string        `yaml:"method"`
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	Token    string        `yaml:"token"`
	APIKey   string        `yaml:"apiKey"`
	Status   *yamlZoneAuth `yaml:"status"`
	Metrics  *yamlZoneAuth `yaml:"metrics"`
}

// yamlZoneAuth is a per-zone auth override in YAML.
type yamlZoneAuth struct {
	Method   string `yaml:"method"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
	APIKey   string `yaml:"apiKey"`
}

// yamlDep describes a single dependency in YAML.
type yamlDep struct {
	Name            string       `yaml:"name"`
	Type            string       `yaml:"type"`
	URL             string       `yaml:"url"`
	Host            string       `yaml:"host"`
	Port            string       `yaml:"port"`
	Critical        bool         `yaml:"critical"`
	HealthPath      string       `yaml:"healthPath"`
	CheckInterval   string       `yaml:"checkInterval"`
	Timeout         string       `yaml:"timeout"`
	TLS             *bool        `yaml:"tls"`
	TLSSkipVerify   *bool        `yaml:"tlsSkipVerify"`
	GRPCServiceName string       `yaml:"grpcServiceName"`
	PostgresQuery   string       `yaml:"postgresQuery"`
	MySQLQuery      string       `yaml:"mysqlQuery"`
	RedisPassword   string       `yaml:"redisPassword"`
	RedisDB         *int         `yaml:"redisDB"`
	AMQPURL         string       `yaml:"amqpUrl"`
	Auth            *yamlDepAuth `yaml:"auth"`
}

// yamlDepAuth holds per-dependency authentication in YAML.
type yamlDepAuth struct {
	BearerToken string            `yaml:"bearerToken"`
	BasicUser   string            `yaml:"basicUser"`
	BasicPass   string            `yaml:"basicPass"`
	Headers     map[string]string `yaml:"headers"`
	Metadata    map[string]string `yaml:"metadata"`
}

// loadFromYAML reads a YAML configuration file and converts it to Config.
func loadFromYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	return convertYAMLToConfig(&yc)
}

// convertYAMLToConfig converts the intermediate yamlConfig to the application Config.
func convertYAMLToConfig(yc *yamlConfig) (*Config, error) {
	cfg := &Config{
		Name:       yc.Name,
		Group:      yc.Group,
		ListenAddr: yc.ListenAddr,
	}

	// Log config.
	cfg.Log = LogConfig{
		Format:     yc.Log.Format,
		Level:      yc.Log.Level,
		TimeFormat: yc.Log.TimeFormat,
		TimeKey:    yc.Log.TimeKey,
		LevelKey:   yc.Log.LevelKey,
		MessageKey: yc.Log.MessageKey,
		SourceKey:  yc.Log.SourceKey,
	}
	if yc.Log.AddSource != nil {
		cfg.Log.AddSource = *yc.Log.AddSource
	}

	// Durations.
	var err error
	cfg.CheckInterval, err = parseDurationFlexible(yc.CheckInterval)
	if err != nil {
		return nil, fmt.Errorf("checkInterval: %w", err)
	}

	cfg.Timeout, err = parseDurationFlexible(yc.Timeout)
	if err != nil {
		return nil, fmt.Errorf("timeout: %w", err)
	}

	cfg.FetchTimeout, err = parseDurationFlexible(yc.FetchTimeout)
	if err != nil {
		return nil, fmt.Errorf("fetchTimeout: %w", err)
	}

	// IsEntry flag.
	if yc.IsEntry != nil {
		cfg.IsEntry = *yc.IsEntry
	}

	// Server auth.
	cfg.Auth = convertYAMLAuth(&yc.Auth)

	// Dependencies.
	for i, yd := range yc.Dependencies {
		dep, err := convertYAMLDep(&yd)
		if err != nil {
			return nil, fmt.Errorf("dependencies[%d] (%s): %w", i, yd.Name, err)
		}
		cfg.Dependencies = append(cfg.Dependencies, dep)
	}

	return cfg, nil
}

// convertYAMLDep converts a single yamlDep to Dependency.
func convertYAMLDep(yd *yamlDep) (Dependency, error) {
	dep := Dependency{
		Name:            yd.Name,
		Type:            yd.Type,
		URL:             yd.URL,
		Host:            yd.Host,
		Port:            yd.Port,
		Critical:        yd.Critical,
		HealthPath:      yd.HealthPath,
		TLS:             yd.TLS,
		TLSSkipVerify:   yd.TLSSkipVerify,
		GRPCServiceName: yd.GRPCServiceName,
		PostgresQuery:   yd.PostgresQuery,
		MySQLQuery:      yd.MySQLQuery,
		RedisPassword:   yd.RedisPassword,
		RedisDB:         yd.RedisDB,
		AMQPURL:         yd.AMQPURL,
	}

	var err error
	dep.CheckInterval, err = parseDurationFlexible(yd.CheckInterval)
	if err != nil {
		return dep, fmt.Errorf("checkInterval: %w", err)
	}

	dep.Timeout, err = parseDurationFlexible(yd.Timeout)
	if err != nil {
		return dep, fmt.Errorf("timeout: %w", err)
	}

	// Dependency auth.
	if yd.Auth != nil {
		dep.BearerToken = yd.Auth.BearerToken
		dep.BasicUser = yd.Auth.BasicUser
		dep.BasicPass = yd.Auth.BasicPass
		dep.Headers = yd.Auth.Headers
		dep.Metadata = yd.Auth.Metadata
	}

	return dep, nil
}

// convertYAMLAuth converts yamlAuthConfig to AuthConfig.
func convertYAMLAuth(ya *yamlAuthConfig) AuthConfig {
	ac := AuthConfig{
		Method:   ya.Method,
		Username: ya.Username,
		Password: ya.Password,
		Token:    ya.Token,
		APIKey:   ya.APIKey,
	}

	if ya.Status != nil {
		ac.Status = &ZoneAuth{
			Method:   ya.Status.Method,
			Username: ya.Status.Username,
			Password: ya.Status.Password,
			Token:    ya.Status.Token,
			APIKey:   ya.Status.APIKey,
		}
	}

	if ya.Metrics != nil {
		ac.Metrics = &ZoneAuth{
			Method:   ya.Metrics.Method,
			Username: ya.Metrics.Username,
			Password: ya.Metrics.Password,
			Token:    ya.Metrics.Token,
			APIKey:   ya.Metrics.APIKey,
		}
	}

	return ac
}

// parseDurationFlexible parses a duration from string.
// Accepts Go duration strings ("15s", "1m30s") and plain numbers (as seconds).
func parseDurationFlexible(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	// Try Go duration format first.
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// Fallback: plain number as seconds.
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: expected Go duration (15s) or seconds (15)", s)
	}
	return time.Duration(sec * float64(time.Second)), nil
}
