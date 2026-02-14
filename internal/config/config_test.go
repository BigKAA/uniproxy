package config

import (
	"os"
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

func TestLoad_NoDeps(t *testing.T) {
	os.Unsetenv("DEPHEALTH_DEPS")
	setEnvs(t, map[string]string{
		"DEPHEALTH_NAME": "app",
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
