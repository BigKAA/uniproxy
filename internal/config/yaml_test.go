package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDurationFlexible(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"go duration seconds", "15s", 15 * time.Second, false},
		{"go duration complex", "1m30s", 90 * time.Second, false},
		{"go duration millis", "500ms", 500 * time.Millisecond, false},
		{"plain number int", "15", 15 * time.Second, false},
		{"plain number float", "2.5", 2500 * time.Millisecond, false},
		{"plain number zero", "0", 0, false},
		{"empty string", "", 0, false},
		{"invalid value", "abc", 0, true},
		{"invalid mixed", "15x", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDurationFlexible(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDurationFlexible(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseDurationFlexible(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromYAML_Full(t *testing.T) {
	yaml := `
name: full-test
listenAddr: ":9090"
log:
  format: json
  level: debug
  timeFormat: unix
  addSource: true
  timeKey: ts
  levelKey: lvl
  messageKey: message
  sourceKey: src
checkInterval: "30s"
timeout: "5s"
fetchTimeout: "10"
dependencies:
  - name: httpbin
    type: http
    url: "http://httpbin.org"
    critical: true
    healthPath: "/health"
    checkInterval: "20s"
    timeout: "3s"
    tls: false
    tlsSkipVerify: true
  - name: mygrpc
    type: grpc
    host: "grpc.example.com"
    port: "443"
    critical: false
    grpcServiceName: "my.Service"
    tls: true
  - name: myredis
    type: redis
    host: "redis.local"
    port: "6379"
    critical: true
    redisPassword: "secret"
    redisDB: 2
  - name: mypg
    type: postgres
    url: "postgres://user:pass@pg:5432/db"
    critical: true
    postgresQuery: "SELECT 1"
  - name: mymysql
    type: mysql
    url: "user:pass@tcp(mysql:3306)/db"
    critical: false
    mysqlQuery: "SELECT 1"
  - name: myamqp
    type: amqp
    host: "rabbitmq.local"
    port: "5672"
    critical: false
    amqpUrl: "amqp://guest:guest@rabbitmq:5672/"
`

	path := writeTestYAML(t, yaml)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}

	// Top-level fields.
	if cfg.Name != "full-test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "full-test")
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, 30*time.Second)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 5*time.Second)
	}
	if cfg.FetchTimeout != 10*time.Second {
		t.Errorf("FetchTimeout = %v, want %v", cfg.FetchTimeout, 10*time.Second)
	}

	// Log config.
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.TimeFormat != "unix" {
		t.Errorf("Log.TimeFormat = %q, want %q", cfg.Log.TimeFormat, "unix")
	}
	if !cfg.Log.AddSource {
		t.Error("Log.AddSource = false, want true")
	}
	if cfg.Log.TimeKey != "ts" {
		t.Errorf("Log.TimeKey = %q, want %q", cfg.Log.TimeKey, "ts")
	}
	if cfg.Log.LevelKey != "lvl" {
		t.Errorf("Log.LevelKey = %q, want %q", cfg.Log.LevelKey, "lvl")
	}
	if cfg.Log.MessageKey != "message" {
		t.Errorf("Log.MessageKey = %q, want %q", cfg.Log.MessageKey, "message")
	}
	if cfg.Log.SourceKey != "src" {
		t.Errorf("Log.SourceKey = %q, want %q", cfg.Log.SourceKey, "src")
	}

	// Dependencies count.
	if len(cfg.Dependencies) != 6 {
		t.Fatalf("Dependencies count = %d, want 6", len(cfg.Dependencies))
	}

	// HTTP dep.
	d := cfg.Dependencies[0]
	if d.Name != "httpbin" || d.Type != "http" {
		t.Errorf("dep[0] = %s:%s, want httpbin:http", d.Name, d.Type)
	}
	if d.URL != "http://httpbin.org" {
		t.Errorf("dep[0].URL = %q", d.URL)
	}
	if !d.Critical {
		t.Error("dep[0].Critical = false, want true")
	}
	if d.HealthPath != "/health" {
		t.Errorf("dep[0].HealthPath = %q", d.HealthPath)
	}
	if d.CheckInterval != 20*time.Second {
		t.Errorf("dep[0].CheckInterval = %v, want 20s", d.CheckInterval)
	}
	if d.Timeout != 3*time.Second {
		t.Errorf("dep[0].Timeout = %v, want 3s", d.Timeout)
	}
	if d.TLS == nil || *d.TLS != false {
		t.Error("dep[0].TLS should be false")
	}
	if d.TLSSkipVerify == nil || *d.TLSSkipVerify != true {
		t.Error("dep[0].TLSSkipVerify should be true")
	}

	// gRPC dep.
	d = cfg.Dependencies[1]
	if d.Name != "mygrpc" || d.Type != "grpc" {
		t.Errorf("dep[1] = %s:%s", d.Name, d.Type)
	}
	if d.Host != "grpc.example.com" || d.Port != "443" {
		t.Errorf("dep[1] host:port = %s:%s", d.Host, d.Port)
	}
	if d.GRPCServiceName != "my.Service" {
		t.Errorf("dep[1].GRPCServiceName = %q", d.GRPCServiceName)
	}
	if d.TLS == nil || *d.TLS != true {
		t.Error("dep[1].TLS should be true")
	}

	// Redis dep.
	d = cfg.Dependencies[2]
	if d.Name != "myredis" || d.Type != "redis" {
		t.Errorf("dep[2] = %s:%s", d.Name, d.Type)
	}
	if d.RedisPassword != "secret" {
		t.Errorf("dep[2].RedisPassword = %q", d.RedisPassword)
	}
	if d.RedisDB == nil || *d.RedisDB != 2 {
		t.Error("dep[2].RedisDB should be 2")
	}

	// Postgres dep.
	d = cfg.Dependencies[3]
	if d.Name != "mypg" || d.Type != "postgres" {
		t.Errorf("dep[3] = %s:%s", d.Name, d.Type)
	}
	if d.PostgresQuery != "SELECT 1" {
		t.Errorf("dep[3].PostgresQuery = %q", d.PostgresQuery)
	}

	// MySQL dep.
	d = cfg.Dependencies[4]
	if d.MySQLQuery != "SELECT 1" {
		t.Errorf("dep[4].MySQLQuery = %q", d.MySQLQuery)
	}

	// AMQP dep.
	d = cfg.Dependencies[5]
	if d.AMQPURL != "amqp://guest:guest@rabbitmq:5672/" {
		t.Errorf("dep[5].AMQPURL = %q", d.AMQPURL)
	}
}

func TestLoadFromYAML_Minimal(t *testing.T) {
	yaml := `
name: minimal
`
	path := writeTestYAML(t, yaml)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}

	if cfg.Name != "minimal" {
		t.Errorf("Name = %q, want %q", cfg.Name, "minimal")
	}
	if cfg.ListenAddr != "" {
		t.Errorf("ListenAddr = %q, want empty", cfg.ListenAddr)
	}
	if cfg.CheckInterval != 0 {
		t.Errorf("CheckInterval = %v, want 0", cfg.CheckInterval)
	}
	if cfg.FetchTimeout != 0 {
		t.Errorf("FetchTimeout = %v, want 0", cfg.FetchTimeout)
	}
	if len(cfg.Dependencies) != 0 {
		t.Errorf("Dependencies count = %d, want 0", len(cfg.Dependencies))
	}
	if cfg.Log.Format != "" {
		t.Errorf("Log.Format = %q, want empty", cfg.Log.Format)
	}
}

func TestLoadFromYAML_DepAuth(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		checkDep func(t *testing.T, dep Dependency)
	}{
		{
			name: "bearer token",
			yaml: `
name: auth-test
dependencies:
  - name: api
    type: http
    url: "http://api.example.com"
    critical: true
    auth:
      bearerToken: "my-secret-token"
`,
			checkDep: func(t *testing.T, dep Dependency) {
				if dep.BearerToken != "my-secret-token" {
					t.Errorf("BearerToken = %q, want %q", dep.BearerToken, "my-secret-token")
				}
			},
		},
		{
			name: "basic auth",
			yaml: `
name: auth-test
dependencies:
  - name: api
    type: http
    url: "http://api.example.com"
    critical: true
    auth:
      basicUser: admin
      basicPass: secret123
`,
			checkDep: func(t *testing.T, dep Dependency) {
				if dep.BasicUser != "admin" {
					t.Errorf("BasicUser = %q", dep.BasicUser)
				}
				if dep.BasicPass != "secret123" {
					t.Errorf("BasicPass = %q", dep.BasicPass)
				}
			},
		},
		{
			name: "custom headers",
			yaml: `
name: auth-test
dependencies:
  - name: api
    type: http
    url: "http://api.example.com"
    critical: true
    auth:
      headers:
        X-Custom: "value1"
        X-Another: "value2"
`,
			checkDep: func(t *testing.T, dep Dependency) {
				if len(dep.Headers) != 2 {
					t.Fatalf("Headers count = %d, want 2", len(dep.Headers))
				}
				if dep.Headers["X-Custom"] != "value1" {
					t.Errorf("Headers[X-Custom] = %q", dep.Headers["X-Custom"])
				}
			},
		},
		{
			name: "grpc metadata",
			yaml: `
name: auth-test
dependencies:
  - name: svc
    type: grpc
    host: "grpc.local"
    port: "443"
    critical: true
    auth:
      metadata:
        x-request-id: "123"
`,
			checkDep: func(t *testing.T, dep Dependency) {
				if len(dep.Metadata) != 1 {
					t.Fatalf("Metadata count = %d, want 1", len(dep.Metadata))
				}
				if dep.Metadata["x-request-id"] != "123" {
					t.Errorf("Metadata[x-request-id] = %q", dep.Metadata["x-request-id"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestYAML(t, tt.yaml)
			cfg, err := loadFromYAML(path)
			if err != nil {
				t.Fatalf("loadFromYAML() error: %v", err)
			}
			if len(cfg.Dependencies) != 1 {
				t.Fatalf("Dependencies count = %d, want 1", len(cfg.Dependencies))
			}
			tt.checkDep(t, cfg.Dependencies[0])
		})
	}
}

func TestLoadFromYAML_InvalidYAML(t *testing.T) {
	path := writeTestYAML(t, `
name: [invalid
  yaml: {broken
`)
	_, err := loadFromYAML(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadFromYAML_FileNotFound(t *testing.T) {
	_, err := loadFromYAML("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadFromYAML_InvalidDuration(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "invalid checkInterval",
			yaml: `
name: test
checkInterval: "abc"
`,
		},
		{
			name: "invalid timeout",
			yaml: `
name: test
timeout: "xyz"
`,
		},
		{
			name: "invalid fetchTimeout",
			yaml: `
name: test
fetchTimeout: "!!!"
`,
		},
		{
			name: "invalid dep checkInterval",
			yaml: `
name: test
dependencies:
  - name: api
    type: http
    url: "http://example.com"
    critical: true
    checkInterval: "bad"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestYAML(t, tt.yaml)
			_, err := loadFromYAML(path)
			if err == nil {
				t.Fatal("expected error for invalid duration, got nil")
			}
		})
	}
}

func TestLoadFromYAML_EmptyFile(t *testing.T) {
	path := writeTestYAML(t, "")
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty", cfg.Name)
	}
}

func TestLoadFromYAML_IsEntry(t *testing.T) {
	yamlContent := `
name: test-app
group: test
isEntry: true
dependencies:
  - name: svc
    type: http
    url: http://svc:8080
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if !cfg.IsEntry {
		t.Error("IsEntry = false, want true")
	}
}

func TestLoadFromYAML_IsEntry_False(t *testing.T) {
	yamlContent := `
name: test-app
group: test
isEntry: false
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if cfg.IsEntry {
		t.Error("IsEntry = true, want false")
	}
}

func TestLoadFromYAML_IsEntry_NotSet(t *testing.T) {
	yamlContent := `
name: test-app
group: test
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if cfg.IsEntry {
		t.Error("IsEntry = true, want false (default)")
	}
}

// --- LDAP YAML tests ---

func TestLoadFromYAML_LDAP_Full(t *testing.T) {
	yamlContent := `
name: test-app
group: test
dependencies:
  - name: corp-ldap
    type: ldap
    url: "ldaps://ad.corp.com:636"
    critical: true
    ldapCheckMethod: simple_bind
    ldapBindDN: "cn=readonly,dc=corp,dc=com"
    ldapBindPassword: "s3cret"
    ldapBaseDN: "dc=corp,dc=com"
    ldapSearchFilter: "(objectClass=*)"
    ldapSearchScope: sub
    ldapStartTLS: true
    ldapTLSSkipVerify: false
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.Name != "corp-ldap" {
		t.Errorf("Name = %q, want %q", dep.Name, "corp-ldap")
	}
	if dep.Type != "ldap" {
		t.Errorf("Type = %q, want %q", dep.Type, "ldap")
	}
	if dep.LDAPCheckMethod != "simple_bind" {
		t.Errorf("LDAPCheckMethod = %q, want %q", dep.LDAPCheckMethod, "simple_bind")
	}
	if dep.LDAPBindDN != "cn=readonly,dc=corp,dc=com" {
		t.Errorf("LDAPBindDN = %q, want %q", dep.LDAPBindDN, "cn=readonly,dc=corp,dc=com")
	}
	if dep.LDAPBindPassword != "s3cret" {
		t.Errorf("LDAPBindPassword = %q, want %q", dep.LDAPBindPassword, "s3cret")
	}
	if dep.LDAPBaseDN != "dc=corp,dc=com" {
		t.Errorf("LDAPBaseDN = %q, want %q", dep.LDAPBaseDN, "dc=corp,dc=com")
	}
	if dep.LDAPSearchFilter != "(objectClass=*)" {
		t.Errorf("LDAPSearchFilter = %q, want %q", dep.LDAPSearchFilter, "(objectClass=*)")
	}
	if dep.LDAPSearchScope != "sub" {
		t.Errorf("LDAPSearchScope = %q, want %q", dep.LDAPSearchScope, "sub")
	}
	if dep.LDAPStartTLS == nil || !*dep.LDAPStartTLS {
		t.Error("LDAPStartTLS should be true")
	}
	if dep.LDAPTLSSkipVerify == nil || *dep.LDAPTLSSkipVerify {
		t.Error("LDAPTLSSkipVerify should be false")
	}
}

func TestLoadFromYAML_LDAP_Minimal(t *testing.T) {
	yamlContent := `
name: test-app
group: test
dependencies:
  - name: myldap
    type: ldap
    url: "ldap://ldap.local:389"
    critical: false
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.Type != "ldap" {
		t.Errorf("Type = %q, want %q", dep.Type, "ldap")
	}
	if dep.LDAPCheckMethod != "" {
		t.Errorf("LDAPCheckMethod = %q, want empty (YAML doesn't set default)", dep.LDAPCheckMethod)
	}
	if dep.LDAPStartTLS != nil {
		t.Error("LDAPStartTLS should be nil when not set")
	}
	if dep.LDAPTLSSkipVerify != nil {
		t.Error("LDAPTLSSkipVerify should be nil when not set")
	}
}

// --- HostHeader / GRPCAuthority YAML tests ---

func TestLoadFromYAML_HostHeader(t *testing.T) {
	yamlContent := `
name: test-app
group: test
dependencies:
  - name: web
    type: http
    url: "http://10.0.0.1:80"
    critical: true
    hostHeader: "myapp.example.com"
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.HostHeader != "myapp.example.com" {
		t.Errorf("HostHeader = %q, want %q", dep.HostHeader, "myapp.example.com")
	}
}

func TestLoadFromYAML_GRPCAuthority(t *testing.T) {
	yamlContent := `
name: test-app
group: test
dependencies:
  - name: svc
    type: grpc
    host: "10.0.0.2"
    port: "443"
    critical: true
    grpcAuthority: "myservice.example.com"
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	if len(cfg.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(cfg.Dependencies))
	}
	dep := cfg.Dependencies[0]
	if dep.GRPCAuthority != "myservice.example.com" {
		t.Errorf("GRPCAuthority = %q, want %q", dep.GRPCAuthority, "myservice.example.com")
	}
}

func TestLoadFromYAML_HostHeaderNotSet(t *testing.T) {
	yamlContent := `
name: test-app
group: test
dependencies:
  - name: web
    type: http
    url: "http://web.svc:8080"
    critical: true
`
	path := writeTestYAML(t, yamlContent)
	cfg, err := loadFromYAML(path)
	if err != nil {
		t.Fatalf("loadFromYAML() error: %v", err)
	}
	dep := cfg.Dependencies[0]
	if dep.HostHeader != "" {
		t.Errorf("HostHeader = %q, want empty", dep.HostHeader)
	}
	if dep.GRPCAuthority != "" {
		t.Errorf("GRPCAuthority = %q, want empty", dep.GRPCAuthority)
	}
}
