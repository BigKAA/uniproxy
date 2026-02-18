package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setEnvs sets env vars and returns a cleanup function.
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestLoad_MinimalValid(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":              "test-app",
		"DEPHEALTH_GROUP":              "test",
		"DEPHEALTH_DEPS":              "svc:http",
		"DEPHEALTH_SVC_URL":           "http://svc.default.svc:8080",
		"DEPHEALTH_SVC_CRITICAL":      "yes",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "test-app" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-app")
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, 10*time.Second)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.Name != "svc" || dep.Type != "http" || dep.URL != "http://svc.default.svc:8080" || !dep.Critical {
		t.Errorf("dep = %+v", dep)
	}
}

func TestLoad_MultipleDeps(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":                    "uniproxy-01",
		"DEPHEALTH_GROUP":                    "test",
		"DEPHEALTH_DEPS":                    "uniproxy-02:http,redis:redis,postgresql:postgres,grpc-stub:grpc",
		"DEPHEALTH_UNIPROXY_02_URL":         "http://uniproxy-02.svc:8080",
		"DEPHEALTH_UNIPROXY_02_CRITICAL":    "yes",
		"DEPHEALTH_UNIPROXY_02_HEALTH_PATH": "/readyz",
		"DEPHEALTH_REDIS_URL":               "redis://redis.svc:6379",
		"DEPHEALTH_REDIS_CRITICAL":          "no",
		"DEPHEALTH_POSTGRESQL_URL":          "postgres://user:pass@pg.svc:5432/db",
		"DEPHEALTH_POSTGRESQL_CRITICAL":     "yes",
		"DEPHEALTH_GRPC_STUB_HOST":          "grpc.svc",
		"DEPHEALTH_GRPC_STUB_PORT":          "9090",
		"DEPHEALTH_GRPC_STUB_CRITICAL":      "no",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Dependencies) != 4 {
		t.Fatalf("len(Dependencies) = %d, want 4", len(cfg.Dependencies))
	}

	// HTTP dep with health path.
	http := cfg.Dependencies[0]
	if http.HealthPath != "/readyz" {
		t.Errorf("http dep HealthPath = %q, want %q", http.HealthPath, "/readyz")
	}
	if !http.Critical {
		t.Error("http dep should be critical")
	}

	// Redis dep.
	redis := cfg.Dependencies[1]
	if redis.Critical {
		t.Error("redis dep should not be critical")
	}

	// gRPC dep via host+port.
	grpc := cfg.Dependencies[3]
	if grpc.Host != "grpc.svc" || grpc.Port != "9090" {
		t.Errorf("grpc dep Host:Port = %s:%s, want grpc.svc:9090", grpc.Host, grpc.Port)
	}
	if grpc.URL != "" {
		t.Errorf("grpc dep URL should be empty, got %q", grpc.URL)
	}
}

func TestLoad_CustomInterval(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":           "app",
		"DEPHEALTH_GROUP":           "test",
		"DEPHEALTH_DEPS":           "svc:http",
		"DEPHEALTH_SVC_URL":        "http://svc:80",
		"DEPHEALTH_SVC_CRITICAL":   "yes",
		"DEPHEALTH_CHECK_INTERVAL": "30",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, 30*time.Second)
	}
}

func TestLoad_MissingName(t *testing.T) {
	// Ensure DEPHEALTH_NAME is not set.
	os.Unsetenv("DEPHEALTH_NAME")
	setEnvs(t, map[string]string{
		"DEPHEALTH_DEPS":         "svc:http",
		"DEPHEALTH_SVC_URL":      "http://svc:80",
		"DEPHEALTH_SVC_CRITICAL": "yes",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DEPHEALTH_NAME")
	}
}

func TestLoad_MissingGroup(t *testing.T) {
	os.Unsetenv("DEPHEALTH_GROUP")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":         "app",
		"DEPHEALTH_DEPS":         "svc:http",
		"DEPHEALTH_SVC_URL":      "http://svc:80",
		"DEPHEALTH_SVC_CRITICAL": "yes",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DEPHEALTH_GROUP")
	}
	if !strings.Contains(err.Error(), "DEPHEALTH_GROUP") {
		t.Errorf("error should mention DEPHEALTH_GROUP, got: %v", err)
	}
}

func TestLoad_GroupFromEnv(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":  "app",
		"DEPHEALTH_GROUP": "backend",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Group != "backend" {
		t.Errorf("Group = %q, want %q", cfg.Group, "backend")
	}
}

func TestLoad_GroupEnvOverridesYAML(t *testing.T) {
	yamlContent := `
name: yaml-app
group: yaml-group
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE":    path,
		"DEPHEALTH_GROUP": "env-group",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Group != "env-group" {
		t.Errorf("Group = %q, want %q (env override)", cfg.Group, "env-group")
	}
}

func TestLoad_NoDeps(t *testing.T) {
	os.Unsetenv("DEPHEALTH_DEPS")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Dependencies) != 0 {
		t.Errorf("len(Dependencies) = %d, want 0", len(cfg.Dependencies))
	}
}

func TestLoad_InvalidDepType(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:mongodb",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestLoad_MissingCritical(t *testing.T) {
	os.Unsetenv("DEPHEALTH_SVC_CRITICAL")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":    "app",
		"DEPHEALTH_GROUP":    "test",
		"DEPHEALTH_DEPS":    "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:80",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing CRITICAL")
	}
}

func TestLoad_MissingURL(t *testing.T) {
	os.Unsetenv("DEPHEALTH_SVC_URL")
	os.Unsetenv("DEPHEALTH_SVC_HOST")
	os.Unsetenv("DEPHEALTH_SVC_PORT")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":         "app",
		"DEPHEALTH_GROUP":         "test",
		"DEPHEALTH_DEPS":         "svc:http",
		"DEPHEALTH_SVC_CRITICAL": "yes",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing URL/HOST+PORT")
	}
}

func TestLoad_NewDepTypes(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "t:tcp,m:mysql,a:amqp,k:kafka",
		"DEPHEALTH_T_HOST":           "tcp.svc",
		"DEPHEALTH_T_PORT":           "9000",
		"DEPHEALTH_T_CRITICAL":       "yes",
		"DEPHEALTH_M_URL":            "mysql://user:pass@mysql.svc:3306/db",
		"DEPHEALTH_M_CRITICAL":       "no",
		"DEPHEALTH_A_HOST":           "amqp.svc",
		"DEPHEALTH_A_PORT":           "5672",
		"DEPHEALTH_A_CRITICAL":       "yes",
		"DEPHEALTH_K_HOST":           "kafka.svc",
		"DEPHEALTH_K_PORT":           "9092",
		"DEPHEALTH_K_CRITICAL":       "no",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Dependencies) != 4 {
		t.Fatalf("len(Dependencies) = %d, want 4", len(cfg.Dependencies))
	}

	types := []string{"tcp", "mysql", "amqp", "kafka"}
	for i, want := range types {
		if cfg.Dependencies[i].Type != want {
			t.Errorf("dep[%d].Type = %q, want %q", i, cfg.Dependencies[i].Type, want)
		}
	}
}

func TestLoad_GlobalTimeout(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":    "app",
		"DEPHEALTH_GROUP":    "test",
		"DEPHEALTH_TIMEOUT": "5",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 5*time.Second)
	}
}

func TestLoad_PerDepOptions(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":                      "app",
		"DEPHEALTH_GROUP":                      "test",
		"DEPHEALTH_DEPS":                      "web:http,rpc:grpc,pg:postgres,r:redis",
		"DEPHEALTH_WEB_URL":                   "https://web.svc:443",
		"DEPHEALTH_WEB_CRITICAL":              "yes",
		"DEPHEALTH_WEB_CHECK_INTERVAL":        "30",
		"DEPHEALTH_WEB_TIMEOUT":               "5",
		"DEPHEALTH_WEB_TLS":                   "true",
		"DEPHEALTH_WEB_TLS_SKIP_VERIFY":       "yes",
		"DEPHEALTH_RPC_HOST":                  "grpc.svc",
		"DEPHEALTH_RPC_PORT":                  "443",
		"DEPHEALTH_RPC_CRITICAL":              "no",
		"DEPHEALTH_RPC_TLS":                   "true",
		"DEPHEALTH_RPC_TLS_SKIP_VERIFY":       "false",
		"DEPHEALTH_RPC_GRPC_SERVICE_NAME":     "myservice",
		"DEPHEALTH_PG_URL":                    "postgres://pg.svc:5432/db",
		"DEPHEALTH_PG_CRITICAL":               "yes",
		"DEPHEALTH_PG_POSTGRES_QUERY":         "SELECT 1",
		"DEPHEALTH_R_HOST":                    "redis.svc",
		"DEPHEALTH_R_PORT":                    "6379",
		"DEPHEALTH_R_CRITICAL":                "no",
		"DEPHEALTH_R_REDIS_PASSWORD":          "secret",
		"DEPHEALTH_R_REDIS_DB":                "2",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Dependencies) != 4 {
		t.Fatalf("len(Dependencies) = %d, want 4", len(cfg.Dependencies))
	}

	// HTTP dep with TLS and timing.
	web := cfg.Dependencies[0]
	if web.CheckInterval != 30*time.Second {
		t.Errorf("web.CheckInterval = %v, want 30s", web.CheckInterval)
	}
	if web.Timeout != 5*time.Second {
		t.Errorf("web.Timeout = %v, want 5s", web.Timeout)
	}
	if web.TLS == nil || !*web.TLS {
		t.Error("web.TLS should be true")
	}
	if web.TLSSkipVerify == nil || !*web.TLSSkipVerify {
		t.Error("web.TLSSkipVerify should be true")
	}

	// gRPC dep.
	rpc := cfg.Dependencies[1]
	if rpc.GRPCServiceName != "myservice" {
		t.Errorf("rpc.GRPCServiceName = %q, want %q", rpc.GRPCServiceName, "myservice")
	}
	if rpc.TLS == nil || !*rpc.TLS {
		t.Error("rpc.TLS should be true")
	}
	if rpc.TLSSkipVerify == nil || *rpc.TLSSkipVerify {
		t.Error("rpc.TLSSkipVerify should be false")
	}

	// Postgres dep.
	pg := cfg.Dependencies[2]
	if pg.PostgresQuery != "SELECT 1" {
		t.Errorf("pg.PostgresQuery = %q, want %q", pg.PostgresQuery, "SELECT 1")
	}

	// Redis dep.
	r := cfg.Dependencies[3]
	if r.RedisPassword != "secret" {
		t.Errorf("r.RedisPassword = %q, want %q", r.RedisPassword, "secret")
	}
	if r.RedisDB == nil || *r.RedisDB != 2 {
		t.Errorf("r.RedisDB = %v, want 2", r.RedisDB)
	}
}

func TestLoad_AMQPOptions(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":           "app",
		"DEPHEALTH_GROUP":           "test",
		"DEPHEALTH_DEPS":           "mq:amqp",
		"DEPHEALTH_MQ_HOST":        "amqp.svc",
		"DEPHEALTH_MQ_PORT":        "5672",
		"DEPHEALTH_MQ_CRITICAL":    "yes",
		"DEPHEALTH_MQ_AMQP_URL":    "amqp://user:pass@amqp.svc:5672/vhost",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.AMQPURL != "amqp://user:pass@amqp.svc:5672/vhost" {
		t.Errorf("AMQPURL = %q", dep.AMQPURL)
	}
}

func TestLoad_MySQLOptions(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "db:mysql",
		"DEPHEALTH_DB_URL":           "mysql://mysql.svc:3306/testdb",
		"DEPHEALTH_DB_CRITICAL":      "yes",
		"DEPHEALTH_DB_MYSQL_QUERY":   "SELECT 1",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.MySQLQuery != "SELECT 1" {
		t.Errorf("MySQLQuery = %q, want %q", dep.MySQLQuery, "SELECT 1")
	}
}

func TestLoad_FetchTimeout_Default(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FetchTimeout != 5*time.Second {
		t.Errorf("FetchTimeout = %v, want %v", cfg.FetchTimeout, 5*time.Second)
	}
}

func TestLoad_FetchTimeout_Custom(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_FETCH_TIMEOUT": "10.5",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Duration(10.5 * float64(time.Second))
	if cfg.FetchTimeout != want {
		t.Errorf("FetchTimeout = %v, want %v", cfg.FetchTimeout, want)
	}
}

func TestLoad_FetchTimeout_Invalid(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_FETCH_TIMEOUT": "abc",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DEPHEALTH_FETCH_TIMEOUT")
	}
}

func TestLoad_FetchTimeout_Negative(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_FETCH_TIMEOUT": "-1",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DEPHEALTH_FETCH_TIMEOUT")
	}
}

func TestLoad_LogConfig_Defaults(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.TimeFormat != "rfc3339nano" {
		t.Errorf("Log.TimeFormat = %q, want %q", cfg.Log.TimeFormat, "rfc3339nano")
	}
	if cfg.Log.AddSource {
		t.Error("Log.AddSource should be false by default")
	}
	if cfg.Log.TimeKey != "" || cfg.Log.LevelKey != "" || cfg.Log.MessageKey != "" || cfg.Log.SourceKey != "" {
		t.Errorf("custom keys should be empty by default, got time=%q level=%q msg=%q source=%q",
			cfg.Log.TimeKey, cfg.Log.LevelKey, cfg.Log.MessageKey, cfg.Log.SourceKey)
	}
}

func TestLoad_LogConfig_AllVars(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":  "app",
		"DEPHEALTH_GROUP":  "test",
		"LOG_FORMAT":      "json",
		"LOG_LEVEL":       "warn",
		"LOG_TIME_FORMAT":  "unix",
		"LOG_ADD_SOURCE":   "yes",
		"LOG_TIME_KEY":     "@timestamp",
		"LOG_LEVEL_KEY":    "severity",
		"LOG_MESSAGE_KEY":  "message",
		"LOG_SOURCE_KEY":   "caller",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "warn")
	}
	if cfg.Log.TimeFormat != "unix" {
		t.Errorf("Log.TimeFormat = %q, want %q", cfg.Log.TimeFormat, "unix")
	}
	if !cfg.Log.AddSource {
		t.Error("Log.AddSource should be true")
	}
	if cfg.Log.TimeKey != "@timestamp" {
		t.Errorf("Log.TimeKey = %q, want %q", cfg.Log.TimeKey, "@timestamp")
	}
	if cfg.Log.LevelKey != "severity" {
		t.Errorf("Log.LevelKey = %q, want %q", cfg.Log.LevelKey, "severity")
	}
	if cfg.Log.MessageKey != "message" {
		t.Errorf("Log.MessageKey = %q, want %q", cfg.Log.MessageKey, "message")
	}
	if cfg.Log.SourceKey != "caller" {
		t.Errorf("Log.SourceKey = %q, want %q", cfg.Log.SourceKey, "caller")
	}
}

func TestLoad_LogConfig_InvalidFormat(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
		"LOG_FORMAT":     "xml",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_FORMAT")
	}
}

func TestLoad_LogConfig_InvalidLevel(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
		"LOG_LEVEL":      "trace",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoad_LogConfig_InvalidTimeFormat(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":  "app",
		"DEPHEALTH_GROUP":  "test",
		"LOG_TIME_FORMAT": "iso8601",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_TIME_FORMAT")
	}
}

func TestLoad_LogConfig_InvalidAddSource(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
		"LOG_ADD_SOURCE": "maybe",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_ADD_SOURCE")
	}
}

func TestLoad_LogConfig_BackwardCompat(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
		"DEPHEALTH_GROUP": "test",
		"LOG_LEVEL":      "debug",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
}

// --- resolveSecret / readSecretFile tests ---

func TestReadSecretFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.txt")
	os.WriteFile(path, []byte("  my-secret-token\n"), 0644)

	val, err := readSecretFile(path, "TEST_VAR_FILE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-secret-token" {
		t.Errorf("readSecretFile = %q, want %q", val, "my-secret-token")
	}
}

func TestReadSecretFile_NotFound(t *testing.T) {
	_, err := readSecretFile("/nonexistent/path/file.txt", "TEST_VAR_FILE")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "TEST_VAR_FILE") {
		t.Errorf("error should mention env key, got: %v", err)
	}
}

func TestResolveSecret_OnlyVar(t *testing.T) {
	t.Setenv("TEST_SECRET", "inline-value")
	os.Unsetenv("TEST_SECRET_FILE")

	val, err := resolveSecret("TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "inline-value" {
		t.Errorf("resolveSecret = %q, want %q", val, "inline-value")
	}
}

func TestResolveSecret_OnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("file-value\n"), 0644)

	os.Unsetenv("TEST_SECRET")
	t.Setenv("TEST_SECRET_FILE", path)

	val, err := resolveSecret("TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "file-value" {
		t.Errorf("resolveSecret = %q, want %q", val, "file-value")
	}
}

func TestResolveSecret_BothSet(t *testing.T) {
	t.Setenv("TEST_SECRET", "inline")
	t.Setenv("TEST_SECRET_FILE", "/some/path")

	_, err := resolveSecret("TEST_SECRET")
	if err == nil {
		t.Fatal("expected error when both VAR and VAR_FILE are set")
	}
	if !strings.Contains(err.Error(), "TEST_SECRET") {
		t.Errorf("error should mention var name, got: %v", err)
	}
}

func TestResolveSecret_NeitherSet(t *testing.T) {
	os.Unsetenv("TEST_SECRET")
	os.Unsetenv("TEST_SECRET_FILE")

	val, err := resolveSecret("TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("resolveSecret = %q, want empty string", val)
	}
}

func TestResolveSecret_FileNotReadable(t *testing.T) {
	os.Unsetenv("TEST_SECRET")
	t.Setenv("TEST_SECRET_FILE", "/nonexistent/secret.txt")

	_, err := resolveSecret("TEST_SECRET")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// --- Auth env var parsing tests ---

func TestLoad_Auth_HTTPBearerToken(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "api:http",
		"DEPHEALTH_API_URL":          "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":     "yes",
		"DEPHEALTH_API_BEARER_TOKEN": "my-token-123",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "my-token-123" {
		t.Errorf("BearerToken = %q, want %q", dep.BearerToken, "my-token-123")
	}
}

func TestLoad_Auth_HTTPBasicAuth(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":            "app",
		"DEPHEALTH_GROUP":            "test",
		"DEPHEALTH_DEPS":            "api:http",
		"DEPHEALTH_API_URL":         "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":    "yes",
		"DEPHEALTH_API_BASIC_USER":  "admin",
		"DEPHEALTH_API_BASIC_PASS":  "p@ssw0rd",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BasicUser != "admin" {
		t.Errorf("BasicUser = %q, want %q", dep.BasicUser, "admin")
	}
	if dep.BasicPass != "p@ssw0rd" {
		t.Errorf("BasicPass = %q, want %q", dep.BasicPass, "p@ssw0rd")
	}
}

func TestLoad_Auth_HTTPHeaders(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_API_HEADERS":   `{"X-API-Key":"abc123","X-Custom":"val"}`,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if len(dep.Headers) != 2 {
		t.Fatalf("len(Headers) = %d, want 2", len(dep.Headers))
	}
	if dep.Headers["X-API-Key"] != "abc123" {
		t.Errorf("Headers[X-API-Key] = %q, want %q", dep.Headers["X-API-Key"], "abc123")
	}
	if dep.Headers["X-Custom"] != "val" {
		t.Errorf("Headers[X-Custom] = %q, want %q", dep.Headers["X-Custom"], "val")
	}
}

func TestLoad_Auth_GRPCBearerToken(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "rpc:grpc",
		"DEPHEALTH_RPC_HOST":         "grpc.svc",
		"DEPHEALTH_RPC_PORT":         "443",
		"DEPHEALTH_RPC_CRITICAL":     "yes",
		"DEPHEALTH_RPC_BEARER_TOKEN": "grpc-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "grpc-token" {
		t.Errorf("BearerToken = %q, want %q", dep.BearerToken, "grpc-token")
	}
}

func TestLoad_Auth_GRPCBasicAuth(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":            "app",
		"DEPHEALTH_GROUP":            "test",
		"DEPHEALTH_DEPS":            "rpc:grpc",
		"DEPHEALTH_RPC_HOST":        "grpc.svc",
		"DEPHEALTH_RPC_PORT":        "443",
		"DEPHEALTH_RPC_CRITICAL":    "yes",
		"DEPHEALTH_RPC_BASIC_USER":  "user",
		"DEPHEALTH_RPC_BASIC_PASS":  "pass",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BasicUser != "user" || dep.BasicPass != "pass" {
		t.Errorf("BasicUser:BasicPass = %q:%q, want user:pass", dep.BasicUser, dep.BasicPass)
	}
}

func TestLoad_Auth_GRPCMetadata(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "rpc:grpc",
		"DEPHEALTH_RPC_HOST":      "grpc.svc",
		"DEPHEALTH_RPC_PORT":      "443",
		"DEPHEALTH_RPC_CRITICAL":  "yes",
		"DEPHEALTH_RPC_METADATA":  `{"x-api-key":"key123"}`,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if len(dep.Metadata) != 1 {
		t.Fatalf("len(Metadata) = %d, want 1", len(dep.Metadata))
	}
	if dep.Metadata["x-api-key"] != "key123" {
		t.Errorf("Metadata[x-api-key] = %q, want %q", dep.Metadata["x-api-key"], "key123")
	}
}

func TestLoad_Auth_BearerTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("file-token\n"), 0644)

	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":                  "app",
		"DEPHEALTH_GROUP":                  "test",
		"DEPHEALTH_DEPS":                  "api:http",
		"DEPHEALTH_API_URL":               "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":          "yes",
		"DEPHEALTH_API_BEARER_TOKEN_FILE": tokenPath,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "file-token" {
		t.Errorf("BearerToken = %q, want %q", dep.BearerToken, "file-token")
	}
}

func TestLoad_Auth_BasicPassFromFile(t *testing.T) {
	dir := t.TempDir()
	passPath := filepath.Join(dir, "pass")
	os.WriteFile(passPath, []byte("  file-pass  \n"), 0644)

	os.Unsetenv("DEPHEALTH_API_BASIC_PASS")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":                "app",
		"DEPHEALTH_GROUP":                "test",
		"DEPHEALTH_DEPS":                "api:http",
		"DEPHEALTH_API_URL":             "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":        "yes",
		"DEPHEALTH_API_BASIC_USER":      "admin",
		"DEPHEALTH_API_BASIC_PASS_FILE": passPath,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BasicPass != "file-pass" {
		t.Errorf("BasicPass = %q, want %q (trimmed)", dep.BasicPass, "file-pass")
	}
}

// --- Auth conflict validation tests ---

func TestLoad_Auth_ConflictBearerAndBasic(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "api:http",
		"DEPHEALTH_API_URL":          "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":     "yes",
		"DEPHEALTH_API_BEARER_TOKEN": "token",
		"DEPHEALTH_API_BASIC_USER":   "user",
		"DEPHEALTH_API_BASIC_PASS":   "pass",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for conflicting bearer + basic auth")
	}
	if !strings.Contains(err.Error(), "BEARER_TOKEN") || !strings.Contains(err.Error(), "BASIC_") {
		t.Errorf("error should mention BEARER_TOKEN and BASIC_, got: %v", err)
	}
}

func TestLoad_Auth_IncompleteBasic_NoPass(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS")
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":            "app",
		"DEPHEALTH_GROUP":            "test",
		"DEPHEALTH_DEPS":            "api:http",
		"DEPHEALTH_API_URL":         "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":    "yes",
		"DEPHEALTH_API_BASIC_USER":  "admin",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for basic user without pass")
	}
	if !strings.Contains(err.Error(), "BASIC_PASS") {
		t.Errorf("error should mention BASIC_PASS, got: %v", err)
	}
}

func TestLoad_Auth_IncompleteBasic_NoUser(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BASIC_USER")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":            "app",
		"DEPHEALTH_GROUP":            "test",
		"DEPHEALTH_DEPS":            "api:http",
		"DEPHEALTH_API_URL":         "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":    "yes",
		"DEPHEALTH_API_BASIC_PASS":  "pass",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for basic pass without user")
	}
	if !strings.Contains(err.Error(), "BASIC_USER") {
		t.Errorf("error should mention BASIC_USER, got: %v", err)
	}
}

func TestLoad_Auth_HeadersOnGRPC(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "rpc:grpc",
		"DEPHEALTH_RPC_HOST":      "grpc.svc",
		"DEPHEALTH_RPC_PORT":      "443",
		"DEPHEALTH_RPC_CRITICAL":  "yes",
		"DEPHEALTH_RPC_HEADERS":   `{"X-Key":"val"}`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for HEADERS on gRPC dep")
	}
	if !strings.Contains(err.Error(), "HEADERS") && !strings.Contains(err.Error(), "HTTP") {
		t.Errorf("error should mention HEADERS/HTTP, got: %v", err)
	}
}

func TestLoad_Auth_MetadataOnHTTP(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_API_METADATA":  `{"x-key":"val"}`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for METADATA on HTTP dep")
	}
	if !strings.Contains(err.Error(), "METADATA") && !strings.Contains(err.Error(), "gRPC") {
		t.Errorf("error should mention METADATA/gRPC, got: %v", err)
	}
}

func TestLoad_Auth_InvalidHeadersJSON(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_API_HEADERS":   `{invalid-json}`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid HEADERS JSON")
	}
}

func TestLoad_Auth_InvalidMetadataJSON(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "rpc:grpc",
		"DEPHEALTH_RPC_HOST":      "grpc.svc",
		"DEPHEALTH_RPC_PORT":      "443",
		"DEPHEALTH_RPC_CRITICAL":  "yes",
		"DEPHEALTH_RPC_METADATA":  `not-json`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid METADATA JSON")
	}
}

func TestLoad_Auth_BothVarAndFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("file-token"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":                  "app",
		"DEPHEALTH_GROUP":                  "test",
		"DEPHEALTH_DEPS":                  "api:http",
		"DEPHEALTH_API_URL":               "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":          "yes",
		"DEPHEALTH_API_BEARER_TOKEN":      "inline-token",
		"DEPHEALTH_API_BEARER_TOKEN_FILE": tokenPath,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when both BEARER_TOKEN and BEARER_TOKEN_FILE are set")
	}
}

// --- Global auth fallback tests ---

func TestLoad_Auth_GlobalBearerApplied(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN")
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_BEARER_TOKEN":  "global-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "global-token" {
		t.Errorf("BearerToken = %q, want %q (from global)", dep.BearerToken, "global-token")
	}
}

func TestLoad_Auth_PerDepOverridesGlobal(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "api:http",
		"DEPHEALTH_API_URL":          "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":     "yes",
		"DEPHEALTH_BEARER_TOKEN":     "global-token",
		"DEPHEALTH_API_BEARER_TOKEN": "per-dep-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "per-dep-token" {
		t.Errorf("BearerToken = %q, want %q (per-dep override)", dep.BearerToken, "per-dep-token")
	}
}

func TestLoad_Auth_GlobalBasicAuthApplied(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BASIC_USER")
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS")
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_BASIC_USER":    "global-user",
		"DEPHEALTH_BASIC_PASS":    "global-pass",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BasicUser != "global-user" || dep.BasicPass != "global-pass" {
		t.Errorf("BasicUser:BasicPass = %q:%q, want global-user:global-pass",
			dep.BasicUser, dep.BasicPass)
	}
}

func TestLoad_Auth_GlobalHeadersHTTPOnly(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_HEADERS")
	os.Unsetenv("DEPHEALTH_RPC_HEADERS")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http,rpc:grpc",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_RPC_HOST":      "grpc.svc",
		"DEPHEALTH_RPC_PORT":      "443",
		"DEPHEALTH_RPC_CRITICAL":  "yes",
		"DEPHEALTH_HEADERS":       `{"X-Global":"val"}`,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpDep := cfg.Dependencies[0]
	if httpDep.Headers["X-Global"] != "val" {
		t.Errorf("HTTP dep Headers = %v, want X-Global:val", httpDep.Headers)
	}

	grpcDep := cfg.Dependencies[1]
	if len(grpcDep.Headers) != 0 {
		t.Errorf("gRPC dep should not have global Headers, got %v", grpcDep.Headers)
	}
}

func TestLoad_Auth_GlobalMetadataGRPCOnly(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_METADATA")
	os.Unsetenv("DEPHEALTH_RPC_METADATA")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":          "app",
		"DEPHEALTH_GROUP":          "test",
		"DEPHEALTH_DEPS":          "api:http,rpc:grpc",
		"DEPHEALTH_API_URL":       "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":  "yes",
		"DEPHEALTH_RPC_HOST":      "grpc.svc",
		"DEPHEALTH_RPC_PORT":      "443",
		"DEPHEALTH_RPC_CRITICAL":  "yes",
		"DEPHEALTH_METADATA":      `{"x-global-key":"val"}`,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpDep := cfg.Dependencies[0]
	if len(httpDep.Metadata) != 0 {
		t.Errorf("HTTP dep should not have global Metadata, got %v", httpDep.Metadata)
	}

	grpcDep := cfg.Dependencies[1]
	if grpcDep.Metadata["x-global-key"] != "val" {
		t.Errorf("gRPC dep Metadata = %v, want x-global-key:val", grpcDep.Metadata)
	}
}

func TestLoad_Auth_PerDepBearerClearsGlobalBasic(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BASIC_USER")
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS")
	os.Unsetenv("DEPHEALTH_API_BASIC_PASS_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":             "app",
		"DEPHEALTH_GROUP":             "test",
		"DEPHEALTH_DEPS":             "api:http",
		"DEPHEALTH_API_URL":          "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":     "yes",
		"DEPHEALTH_BASIC_USER":       "global-user",
		"DEPHEALTH_BASIC_PASS":       "global-pass",
		"DEPHEALTH_API_BEARER_TOKEN": "per-dep-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "per-dep-token" {
		t.Errorf("BearerToken = %q, want %q", dep.BearerToken, "per-dep-token")
	}
	if dep.BasicUser != "" || dep.BasicPass != "" {
		t.Errorf("global basic auth should be cleared, got user=%q pass=%q", dep.BasicUser, dep.BasicPass)
	}
}

func TestLoad_Auth_PerDepBasicClearsGlobalBearer(t *testing.T) {
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN")
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":            "app",
		"DEPHEALTH_GROUP":            "test",
		"DEPHEALTH_DEPS":            "api:http",
		"DEPHEALTH_API_URL":         "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":    "yes",
		"DEPHEALTH_BEARER_TOKEN":    "global-token",
		"DEPHEALTH_API_BASIC_USER":  "per-dep-user",
		"DEPHEALTH_API_BASIC_PASS":  "per-dep-pass",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "" {
		t.Errorf("global bearer should be cleared, got %q", dep.BearerToken)
	}
	if dep.BasicUser != "per-dep-user" || dep.BasicPass != "per-dep-pass" {
		t.Errorf("BasicUser:BasicPass = %q:%q, want per-dep-user:per-dep-pass",
			dep.BasicUser, dep.BasicPass)
	}
}

func TestLoad_Auth_GlobalBearerTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "global-token")
	os.WriteFile(tokenPath, []byte("global-file-token\n"), 0644)

	os.Unsetenv("DEPHEALTH_BEARER_TOKEN")
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN")
	os.Unsetenv("DEPHEALTH_API_BEARER_TOKEN_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":               "app",
		"DEPHEALTH_GROUP":               "test",
		"DEPHEALTH_DEPS":               "api:http",
		"DEPHEALTH_API_URL":            "http://api.svc:8080",
		"DEPHEALTH_API_CRITICAL":       "yes",
		"DEPHEALTH_BEARER_TOKEN_FILE":  tokenPath,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.BearerToken != "global-file-token" {
		t.Errorf("BearerToken = %q, want %q (from global file)", dep.BearerToken, "global-file-token")
	}
}

func TestLoad_Auth_InvalidGlobalHeadersJSON(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":     "app",
		"DEPHEALTH_GROUP":     "test",
		"DEPHEALTH_HEADERS":  `{bad`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid global HEADERS JSON")
	}
}

func TestLoad_Auth_InvalidGlobalMetadataJSON(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":     "app",
		"DEPHEALTH_GROUP":     "test",
		"DEPHEALTH_METADATA": `[not-an-object]`,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid global METADATA JSON")
	}
}

func TestLoad_Auth_GlobalBothTokenAndFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("token"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":              "app",
		"DEPHEALTH_GROUP":              "test",
		"DEPHEALTH_BEARER_TOKEN":      "inline",
		"DEPHEALTH_BEARER_TOKEN_FILE": tokenPath,
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when both global BEARER_TOKEN and BEARER_TOKEN_FILE are set")
	}
}

// --- validateAuth direct tests ---

func TestValidateAuth_NoAuth(t *testing.T) {
	dep := &Dependency{Name: "test", Type: "http"}
	if err := validateAuth(dep); err != nil {
		t.Errorf("unexpected error for no auth: %v", err)
	}
}

func TestValidateAuth_BearerOnly(t *testing.T) {
	dep := &Dependency{Name: "test", Type: "http", BearerToken: "tok"}
	if err := validateAuth(dep); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAuth_BasicOnly(t *testing.T) {
	dep := &Dependency{Name: "test", Type: "http", BasicUser: "u", BasicPass: "p"}
	if err := validateAuth(dep); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAuth_HeadersOnHTTP(t *testing.T) {
	dep := &Dependency{Name: "test", Type: "http", Headers: map[string]string{"K": "V"}}
	if err := validateAuth(dep); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAuth_MetadataOnGRPC(t *testing.T) {
	dep := &Dependency{Name: "test", Type: "grpc", Metadata: map[string]string{"k": "v"}}
	if err := validateAuth(dep); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnvName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"uniproxy-02", "UNIPROXY_02"},
		{"redis", "REDIS"},
		{"grpc-stub", "GRPC_STUB"},
		{"postgresql", "POSTGRESQL"},
		{"my-long-service-name", "MY_LONG_SERVICE_NAME"},
	}
	for _, tt := range tests {
		got := EnvName(tt.input)
		if got != tt.want {
			t.Errorf("EnvName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- YAML + Env merge tests (Phase 2.5) ---

func TestLoad_ConfigFile_FullYAML(t *testing.T) {
	yamlContent := `
name: yaml-app
group: test
listenAddr: ":9090"
log:
  format: json
  level: debug
  timeFormat: unix
checkInterval: "30s"
fetchTimeout: "10"
dependencies:
  - name: httpbin
    type: http
    url: "http://httpbin.org"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "yaml-app" {
		t.Errorf("Name = %q, want %q", cfg.Name, "yaml-app")
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.TimeFormat != "unix" {
		t.Errorf("Log.TimeFormat = %q, want %q", cfg.Log.TimeFormat, "unix")
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", cfg.CheckInterval)
	}
	if cfg.FetchTimeout != 10*time.Second {
		t.Errorf("FetchTimeout = %v, want 10s", cfg.FetchTimeout)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("Dependencies count = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.Name != "httpbin" || dep.Type != "http" || dep.URL != "http://httpbin.org" || !dep.Critical {
		t.Errorf("dep = %+v", dep)
	}
}

func TestLoad_ConfigFile_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
name: yaml-name
group: test
listenAddr: ":9090"
log:
  format: text
  level: info
checkInterval: "30s"
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE":            path,
		"DEPHEALTH_NAME":        "env-name",
		"DEPHEALTH_GROUP":        "test",
		"LISTEN_ADDR":           ":7070",
		"LOG_FORMAT":            "json",
		"LOG_LEVEL":             "warn",
		"DEPHEALTH_CHECK_INTERVAL": "60",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Env overrides YAML.
	if cfg.Name != "env-name" {
		t.Errorf("Name = %q, want %q (env override)", cfg.Name, "env-name")
	}
	if cfg.ListenAddr != ":7070" {
		t.Errorf("ListenAddr = %q, want %q (env override)", cfg.ListenAddr, ":7070")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q (env override)", cfg.Log.Format, "json")
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q (env override)", cfg.Log.Level, "warn")
	}
	if cfg.CheckInterval != 60*time.Second {
		t.Errorf("CheckInterval = %v, want 60s (env override)", cfg.CheckInterval)
	}
}

func TestLoad_ConfigFile_EnvDepsReplaceYAMLDeps(t *testing.T) {
	yamlContent := `
name: yaml-app
group: test
dependencies:
  - name: yaml-svc
    type: http
    url: "http://yaml-svc.local"
    critical: true
  - name: yaml-redis
    type: redis
    host: "redis.local"
    port: "6379"
    critical: false
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE":            path,
		"DEPHEALTH_DEPS":        "env-svc:http",
		"DEPHEALTH_ENV_SVC_URL":      "http://env-svc.local",
		"DEPHEALTH_ENV_SVC_CRITICAL": "yes",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DEPHEALTH_DEPS completely replaces YAML deps.
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("Dependencies count = %d, want 1 (env replaces yaml)", len(cfg.Dependencies))
	}
	if cfg.Dependencies[0].Name != "env-svc" {
		t.Errorf("dep[0].Name = %q, want %q", cfg.Dependencies[0].Name, "env-svc")
	}
}

func TestLoad_ConfigFile_PerDepAuthOverlayOnYAMLDeps(t *testing.T) {
	yamlContent := `
name: yaml-app
group: test
dependencies:
  - name: api
    type: http
    url: "http://api.local"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	os.Unsetenv("DEPHEALTH_DEPS")
	setEnvs(t, map[string]string{
		"CONFIG_FILE":                path,
		"DEPHEALTH_API_BEARER_TOKEN": "injected-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("Dependencies count = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.Name != "api" {
		t.Errorf("dep.Name = %q, want %q", dep.Name, "api")
	}
	if dep.BearerToken != "injected-token" {
		t.Errorf("dep.BearerToken = %q, want %q (env overlay on yaml dep)", dep.BearerToken, "injected-token")
	}
}

func TestLoad_NoConfigFile_Regression(t *testing.T) {
	// Without CONFIG_FILE, behavior must be identical to v0.4.x.
	os.Unsetenv("CONFIG_FILE")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME":         "regression-app",
		"DEPHEALTH_GROUP":         "test",
		"DEPHEALTH_DEPS":         "svc:http",
		"DEPHEALTH_SVC_URL":      "http://svc:80",
		"DEPHEALTH_SVC_CRITICAL": "yes",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "regression-app" {
		t.Errorf("Name = %q, want %q", cfg.Name, "regression-app")
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q (default)", cfg.ListenAddr, ":8080")
	}
	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("CheckInterval = %v, want 10s (default)", cfg.CheckInterval)
	}
	if cfg.FetchTimeout != 5*time.Second {
		t.Errorf("FetchTimeout = %v, want 5s (default)", cfg.FetchTimeout)
	}
}

func TestLoad_ConfigFile_NotFound(t *testing.T) {
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("DEPHEALTH_NAME", "app")
	t.Setenv("DEPHEALTH_GROUP", "test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), "config file") {
		t.Errorf("error should mention config file, got: %v", err)
	}
}

func TestLoad_ConfigFile_YAMLDefaults(t *testing.T) {
	// YAML with only name — defaults should be applied.
	yamlContent := `
name: minimal-yaml
group: test
`
	path := writeTestYAML(t, yamlContent)
	os.Unsetenv("DEPHEALTH_DEPS")
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q (default)", cfg.ListenAddr, ":8080")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q (default)", cfg.Log.Format, "text")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q (default)", cfg.Log.Level, "info")
	}
	if cfg.Log.TimeFormat != "rfc3339nano" {
		t.Errorf("Log.TimeFormat = %q, want %q (default)", cfg.Log.TimeFormat, "rfc3339nano")
	}
	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("CheckInterval = %v, want 10s (default)", cfg.CheckInterval)
	}
	if cfg.FetchTimeout != 5*time.Second {
		t.Errorf("FetchTimeout = %v, want 5s (default)", cfg.FetchTimeout)
	}
}

// --- Phase 3: Server Auth Config Tests ---

func TestResolveZone_GlobalMethod(t *testing.T) {
	ac := AuthConfig{
		Method: "bearer",
		Token:  "my-token",
	}
	// Both zones should inherit global.
	for _, zone := range []string{"status", "metrics"} {
		za := ac.ResolveZone(zone)
		if za.Method != "bearer" {
			t.Errorf("ResolveZone(%q).Method = %q, want %q", zone, za.Method, "bearer")
		}
		if za.Token != "my-token" {
			t.Errorf("ResolveZone(%q).Token = %q, want %q", zone, za.Token, "my-token")
		}
	}
}

func TestResolveZone_ZoneOverride(t *testing.T) {
	ac := AuthConfig{
		Method: "bearer",
		Token:  "global-token",
		Status: &ZoneAuth{
			Method:   "basic",
			Username: "admin",
			Password: "secret",
		},
	}
	// Status should be overridden.
	za := ac.ResolveZone("status")
	if za.Method != "basic" {
		t.Errorf("status Method = %q, want %q", za.Method, "basic")
	}
	if za.Username != "admin" {
		t.Errorf("status Username = %q, want %q", za.Username, "admin")
	}
	if za.Password != "secret" {
		t.Errorf("status Password = %q, want %q", za.Password, "secret")
	}

	// Metrics should use global.
	za = ac.ResolveZone("metrics")
	if za.Method != "bearer" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "bearer")
	}
	if za.Token != "global-token" {
		t.Errorf("metrics Token = %q, want %q", za.Token, "global-token")
	}
}

func TestResolveZone_NoneOverride(t *testing.T) {
	ac := AuthConfig{
		Method: "bearer",
		Token:  "my-token",
		Metrics: &ZoneAuth{
			Method: "none",
		},
	}
	// Status inherits global bearer.
	za := ac.ResolveZone("status")
	if za.Method != "bearer" {
		t.Errorf("status Method = %q, want %q", za.Method, "bearer")
	}
	// Metrics overridden to none.
	za = ac.ResolveZone("metrics")
	if za.Method != "none" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "none")
	}
}

func TestResolveZone_EmptyMethod_DefaultsNone(t *testing.T) {
	ac := AuthConfig{}
	za := ac.ResolveZone("status")
	if za.Method != "none" {
		t.Errorf("Method = %q, want %q", za.Method, "none")
	}
}

func TestResolveZone_UnknownZone(t *testing.T) {
	ac := AuthConfig{Method: "bearer", Token: "t"}
	za := ac.ResolveZone("unknown")
	if za.Method != "bearer" {
		t.Errorf("Method = %q, want %q (global fallback)", za.Method, "bearer")
	}
}

func TestLoad_ServerAuth_BearerEnv(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "bearer",
		"AUTH_TOKEN":  "secret-token",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Method != "bearer" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "bearer")
	}
	if cfg.Auth.Token != "secret-token" {
		t.Errorf("Auth.Token = %q, want %q", cfg.Auth.Token, "secret-token")
	}
}

func TestLoad_ServerAuth_BasicEnv(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "basic",
		"AUTH_USER":   "admin",
		"AUTH_PASS":   "password123",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Method != "basic" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "basic")
	}
	if cfg.Auth.Username != "admin" {
		t.Errorf("Auth.Username = %q, want %q", cfg.Auth.Username, "admin")
	}
	if cfg.Auth.Password != "password123" {
		t.Errorf("Auth.Password = %q, want %q", cfg.Auth.Password, "password123")
	}
}

func TestLoad_ServerAuth_APIKeyEnv(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":  "apikey",
		"AUTH_API_KEY": "my-api-key",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Method != "apikey" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "apikey")
	}
	if cfg.Auth.APIKey != "my-api-key" {
		t.Errorf("Auth.APIKey = %q, want %q", cfg.Auth.APIKey, "my-api-key")
	}
}

func TestLoad_ServerAuth_PerZoneEnv(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":         "bearer",
		"AUTH_TOKEN":          "global-token",
		"AUTH_STATUS_METHOD":  "basic",
		"AUTH_STATUS_USER":    "admin",
		"AUTH_STATUS_PASS":    "secret",
		"AUTH_METRICS_METHOD": "none",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Status zone should be basic.
	za := cfg.Auth.ResolveZone("status")
	if za.Method != "basic" {
		t.Errorf("status Method = %q, want %q", za.Method, "basic")
	}
	if za.Username != "admin" {
		t.Errorf("status Username = %q, want %q", za.Username, "admin")
	}

	// Metrics zone should be none (override).
	za = cfg.Auth.ResolveZone("metrics")
	if za.Method != "none" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "none")
	}
}

func TestLoad_ServerAuth_TokenFromFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("file-token\n"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":     "bearer",
		"AUTH_TOKEN_FILE": tokenFile,
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Token != "file-token" {
		t.Errorf("Auth.Token = %q, want %q", cfg.Auth.Token, "file-token")
	}
}

func TestLoad_ServerAuth_PassFromFile(t *testing.T) {
	dir := t.TempDir()
	passFile := filepath.Join(dir, "pass")
	os.WriteFile(passFile, []byte("file-pass\n"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":    "basic",
		"AUTH_USER":      "admin",
		"AUTH_PASS_FILE": passFile,
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Password != "file-pass" {
		t.Errorf("Auth.Password = %q, want %q", cfg.Auth.Password, "file-pass")
	}
}

func TestLoad_ServerAuth_APIKeyFromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "apikey")
	os.WriteFile(keyFile, []byte("file-key\n"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":       "apikey",
		"AUTH_API_KEY_FILE": keyFile,
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.APIKey != "file-key" {
		t.Errorf("Auth.APIKey = %q, want %q", cfg.Auth.APIKey, "file-key")
	}
}

func TestLoad_ServerAuth_BothVarAndFile_Error(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("file-token"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":     "bearer",
		"AUTH_TOKEN":      "inline-token",
		"AUTH_TOKEN_FILE": tokenFile,
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when both AUTH_TOKEN and AUTH_TOKEN_FILE are set")
	}
}

func TestLoad_ServerAuth_BasicMissingPass_Error(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "basic",
		"AUTH_USER":   "admin",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for basic auth without password")
	}
}

func TestLoad_ServerAuth_BasicMissingUser_Error(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "basic",
		"AUTH_PASS":   "secret",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for basic auth without username")
	}
}

func TestLoad_ServerAuth_BearerMissingToken_Error(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "bearer",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for bearer auth without token")
	}
}

func TestLoad_ServerAuth_APIKeyMissing_Error(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "apikey",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for apikey auth without api_key")
	}
}

func TestLoad_ServerAuth_UnknownMethod_Error(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD": "oauth2",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unknown auth method")
	}
}

func TestLoad_ServerAuth_NoAuthVars_DefaultNone(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Method != "none" {
		t.Errorf("Auth.Method = %q, want %q (default)", cfg.Auth.Method, "none")
	}
}

func TestLoad_ServerAuth_YAMLAuth(t *testing.T) {
	yamlContent := `
name: yaml-auth
group: test
auth:
  method: bearer
  token: yaml-token
  status:
    method: basic
    username: admin
    password: secret
  metrics:
    method: none
dependencies:
  - name: svc
    type: http
    url: "http://svc:8080"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Method != "bearer" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "bearer")
	}
	if cfg.Auth.Token != "yaml-token" {
		t.Errorf("Auth.Token = %q, want %q", cfg.Auth.Token, "yaml-token")
	}

	// Status zone: basic.
	za := cfg.Auth.ResolveZone("status")
	if za.Method != "basic" {
		t.Errorf("status Method = %q, want %q", za.Method, "basic")
	}
	if za.Username != "admin" {
		t.Errorf("status Username = %q, want %q", za.Username, "admin")
	}
	if za.Password != "secret" {
		t.Errorf("status Password = %q, want %q", za.Password, "secret")
	}

	// Metrics zone: none (override).
	za = cfg.Auth.ResolveZone("metrics")
	if za.Method != "none" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "none")
	}
}

func TestLoad_ServerAuth_YAMLWithEnvOverride(t *testing.T) {
	yamlContent := `
name: yaml-auth
group: test
auth:
  method: bearer
  token: yaml-token
dependencies:
  - name: svc
    type: http
    url: "http://svc:8080"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE":        path,
		"AUTH_STATUS_METHOD": "basic",
		"AUTH_STATUS_USER":   "env-admin",
		"AUTH_STATUS_PASS":   "env-secret",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Global still bearer from YAML.
	if cfg.Auth.Method != "bearer" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "bearer")
	}

	// Status should be basic from env override.
	za := cfg.Auth.ResolveZone("status")
	if za.Method != "basic" {
		t.Errorf("status Method = %q, want %q", za.Method, "basic")
	}
	if za.Username != "env-admin" {
		t.Errorf("status Username = %q, want %q", za.Username, "env-admin")
	}

	// Metrics should fall back to global bearer.
	za = cfg.Auth.ResolveZone("metrics")
	if za.Method != "bearer" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "bearer")
	}
}

func TestLoad_ServerAuth_GlobalBearerZoneMetricsNone(t *testing.T) {
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_METHOD":         "bearer",
		"AUTH_TOKEN":          "my-token",
		"AUTH_METRICS_METHOD": "none",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Status inherits global bearer.
	za := cfg.Auth.ResolveZone("status")
	if za.Method != "bearer" {
		t.Errorf("status Method = %q, want %q", za.Method, "bearer")
	}
	if za.Token != "my-token" {
		t.Errorf("status Token = %q, want %q", za.Token, "my-token")
	}

	// Metrics overridden to none.
	za = cfg.Auth.ResolveZone("metrics")
	if za.Method != "none" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "none")
	}
}

func TestLoad_ServerAuth_PerZoneTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "status-token")
	os.WriteFile(tokenFile, []byte("zone-token\n"), 0644)

	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "test-app",
		"DEPHEALTH_GROUP": "test",
		"DEPHEALTH_DEPS": "svc:http",
		"DEPHEALTH_SVC_URL": "http://svc:8080",
		"DEPHEALTH_SVC_CRITICAL": "yes",
		"AUTH_STATUS_METHOD":     "bearer",
		"AUTH_STATUS_TOKEN_FILE": tokenFile,
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	za := cfg.Auth.ResolveZone("status")
	if za.Method != "bearer" {
		t.Errorf("status Method = %q, want %q", za.Method, "bearer")
	}
	if za.Token != "zone-token" {
		t.Errorf("status Token = %q, want %q", za.Token, "zone-token")
	}
}

func TestLoad_ServerAuth_YAMLZoneWithEnvOverlay(t *testing.T) {
	// YAML sets status zone with basic auth.
	// Env overrides only the password — other fields should stay from YAML.
	yamlContent := `
name: yaml-overlay
group: test
auth:
  method: bearer
  token: global-token
  status:
    method: basic
    username: yaml-user
    password: yaml-pass
  metrics:
    method: apikey
    apiKey: yaml-key
dependencies:
  - name: svc
    type: http
    url: "http://svc:8080"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE":         path,
		"AUTH_STATUS_PASS":    "env-pass",
		"AUTH_METRICS_METHOD": "bearer",
		"AUTH_METRICS_TOKEN":  "env-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Status: method and username from YAML, password from env.
	za := cfg.Auth.ResolveZone("status")
	if za.Method != "basic" {
		t.Errorf("status Method = %q, want %q", za.Method, "basic")
	}
	if za.Username != "yaml-user" {
		t.Errorf("status Username = %q, want %q", za.Username, "yaml-user")
	}
	if za.Password != "env-pass" {
		t.Errorf("status Password = %q, want %q", za.Password, "env-pass")
	}

	// Metrics: method and token from env, apiKey from YAML stays.
	za = cfg.Auth.ResolveZone("metrics")
	if za.Method != "bearer" {
		t.Errorf("metrics Method = %q, want %q", za.Method, "bearer")
	}
	if za.Token != "env-token" {
		t.Errorf("metrics Token = %q, want %q", za.Token, "env-token")
	}
}

func TestLoad_ServerAuth_EnvGlobalOverridesYAMLGlobal(t *testing.T) {
	yamlContent := `
name: yaml-global
group: test
auth:
  method: basic
  username: yaml-user
  password: yaml-pass
dependencies:
  - name: svc
    type: http
    url: "http://svc:8080"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	setEnvs(t, map[string]string{
		"CONFIG_FILE": path,
		"AUTH_METHOD": "bearer",
		"AUTH_TOKEN":  "env-token",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Auth.Method != "bearer" {
		t.Errorf("Auth.Method = %q, want %q", cfg.Auth.Method, "bearer")
	}
	if cfg.Auth.Token != "env-token" {
		t.Errorf("Auth.Token = %q, want %q", cfg.Auth.Token, "env-token")
	}
}
