package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"

	"github.com/BigKAA/uniproxy/internal/config"
)

// mockHealthChecker implements HealthChecker for testing.
type mockHealthChecker struct {
	health  map[string]bool
	details map[string]dephealth.EndpointStatus
}

func (m *mockHealthChecker) Health() map[string]bool {
	return m.health
}

func (m *mockHealthChecker) HealthDetails() map[string]dephealth.EndpointStatus {
	return m.details
}

func boolPtr(b bool) *bool { return &b }

func newTestServer(mock *mockHealthChecker) *Server {
	return New(mock, "test-app", 5*time.Second, config.AuthConfig{})
}

func TestHandleRoot_SimpleFormat(t *testing.T) {
	mock := &mockHealthChecker{
		health: map[string]bool{
			"svc:host:80": true,
			"db:pg:5432":  false,
		},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "test-app" {
		t.Errorf("Name = %q, want %q", resp.Name, "test-app")
	}
	if len(resp.Health) != 2 {
		t.Errorf("len(Health) = %d, want 2", len(resp.Health))
	}
	if !resp.Health["svc:host:80"] {
		t.Error("svc:host:80 should be true")
	}
	if resp.Health["db:pg:5432"] {
		t.Error("db:pg:5432 should be false")
	}
}

func TestHandleRoot_SimpleFormat_NoDetailParam(t *testing.T) {
	// detail=false or detail=abc should return simple format.
	tests := []string{
		"/",
		"/?detail=false",
		"/?detail=abc",
		"/?detail=",
	}
	mock := &mockHealthChecker{
		health: map[string]bool{"svc:h:80": true},
	}
	srv := newTestServer(mock)

	for _, url := range tests {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		var resp map[string]json.RawMessage
		_ = json.NewDecoder(rec.Body).Decode(&resp)
		if _, ok := resp["health"]; !ok {
			t.Errorf("URL %q: expected 'health' field (simple format)", url)
		}
		if _, ok := resp["dependencies"]; ok {
			t.Errorf("URL %q: unexpected 'dependencies' field (should be simple format)", url)
		}
	}
}

func TestHandleRoot_DetailFormat(t *testing.T) {
	checkedAt := time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC)
	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{
			"httpbin:httpbin.org:80": {
				Healthy:       boolPtr(true),
				Status:        dephealth.StatusOK,
				Detail:        "200_ok",
				Latency:       45 * time.Millisecond,
				Type:          dephealth.TypeHTTP,
				Name:          "httpbin",
				Host:          "httpbin.org",
				Port:          "80",
				Critical:      true,
				LastCheckedAt: checkedAt,
				Labels:        map[string]string{},
			},
			"db:pg.svc:5432": {
				Healthy:       boolPtr(false),
				Status:        dephealth.StatusConnectionError,
				Detail:        "connection_refused",
				Latency:       0,
				Type:          dephealth.TypePostgres,
				Name:          "db",
				Host:          "pg.svc",
				Port:          "5432",
				Critical:      true,
				LastCheckedAt: checkedAt,
				Labels:        map[string]string{},
			},
		},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp DetailStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "test-app" {
		t.Errorf("Name = %q, want %q", resp.Name, "test-app")
	}
	if len(resp.Dependencies) != 2 {
		t.Fatalf("len(Dependencies) = %d, want 2", len(resp.Dependencies))
	}

	// Check HTTP dependency.
	httpDep := resp.Dependencies["httpbin:httpbin.org:80"]
	if httpDep == nil {
		t.Fatal("missing dependency httpbin:httpbin.org:80")
	}
	if httpDep.Healthy == nil || !*httpDep.Healthy {
		t.Error("httpbin should be healthy")
	}
	if httpDep.Status != dephealth.StatusOK {
		t.Errorf("httpbin Status = %q, want %q", httpDep.Status, dephealth.StatusOK)
	}
	if httpDep.Type != dephealth.TypeHTTP {
		t.Errorf("httpbin Type = %q, want %q", httpDep.Type, dephealth.TypeHTTP)
	}
	if httpDep.Name != "httpbin" {
		t.Errorf("httpbin Name = %q, want %q", httpDep.Name, "httpbin")
	}
	if httpDep.Response != nil {
		t.Error("httpbin Response should be nil (no fetch in Phase 2)")
	}

	// Check Postgres dependency.
	pgDep := resp.Dependencies["db:pg.svc:5432"]
	if pgDep == nil {
		t.Fatal("missing dependency db:pg.svc:5432")
	}
	if pgDep.Healthy == nil || *pgDep.Healthy {
		t.Error("db should be unhealthy")
	}
	if pgDep.Status != dephealth.StatusConnectionError {
		t.Errorf("db Status = %q, want %q", pgDep.Status, dephealth.StatusConnectionError)
	}
}

func TestHandleRoot_DetailFormat_JSONFields(t *testing.T) {
	// Verify raw JSON contains expected field names (latency_ms, last_checked_at, etc.).
	checkedAt := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{
			"svc:h:80": {
				Healthy:       boolPtr(true),
				Status:        dephealth.StatusOK,
				Detail:        "ok",
				Latency:       123456 * time.Microsecond, // 123.456 ms
				Type:          dephealth.TypeHTTP,
				Name:          "svc",
				Host:          "h",
				Port:          "80",
				LastCheckedAt: checkedAt,
				Labels:        map[string]string{},
			},
		},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var raw map[string]json.RawMessage
	_ = json.NewDecoder(rec.Body).Decode(&raw)

	var deps map[string]json.RawMessage
	_ = json.Unmarshal(raw["dependencies"], &deps)
	depJSON := string(deps["svc:h:80"])

	// Check for SDK's latency_ms format.
	var depFields map[string]json.RawMessage
	_ = json.Unmarshal(deps["svc:h:80"], &depFields)

	if _, ok := depFields["latency_ms"]; !ok {
		t.Errorf("missing latency_ms field in JSON: %s", depJSON)
	}
	if _, ok := depFields["last_checked_at"]; !ok {
		t.Errorf("missing last_checked_at field in JSON: %s", depJSON)
	}
	if _, ok := depFields["labels"]; !ok {
		t.Errorf("missing labels field in JSON: %s", depJSON)
	}
	// response should NOT be present when nil (omitempty).
	if _, ok := depFields["response"]; ok {
		t.Errorf("unexpected response field in JSON when nil: %s", depJSON)
	}
}

func TestDependencyDetail_MarshalJSON_WithResponse(t *testing.T) {
	// Verify response field appears in JSON when set.
	respData := json.RawMessage(`{"name":"downstream"}`)
	es := dephealth.EndpointStatus{
		Healthy: boolPtr(true),
		Status:  dephealth.StatusOK,
		Detail:  "ok",
		Type:    dephealth.TypeHTTP,
		Name:    "svc",
		Host:    "h",
		Port:    "80",
		Labels:  map[string]string{},
	}

	dd := &DependencyDetail{
		EndpointStatus: es,
		Response:       &respData,
	}
	data, err := json.Marshal(dd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	if _, ok := fields["response"]; !ok {
		t.Error("response field missing when set")
	}
	if string(fields["response"]) != `{"name":"downstream"}` {
		t.Errorf("response = %s, want {\"name\":\"downstream\"}", string(fields["response"]))
	}
	if _, ok := fields["latency_ms"]; !ok {
		t.Error("latency_ms missing after merge")
	}
}

func TestParseDepth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 1},     // default
		{"0", 0},    // explicit 0
		{"1", 1},    // explicit 1
		{"3", 3},    // explicit 3
		{"10", 10},  // max
		{"11", 10},  // capped at 10
		{"100", 10}, // capped at 10
		{"-1", 1},   // negative → default
		{"abc", 1},  // invalid → default
		{"-5", 1},   // negative → default
	}
	for _, tt := range tests {
		got := parseDepth(tt.input)
		if got != tt.want {
			t.Errorf("parseDepth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestHandleHealthz(t *testing.T) {
	srv := newTestServer(&mockHealthChecker{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestHandleReadyz(t *testing.T) {
	srv := newTestServer(&mockHealthChecker{})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestHandleRoot_DetailFormat_EmptyDeps(t *testing.T) {
	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp DetailStatusResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Dependencies) != 0 {
		t.Errorf("len(Dependencies) = %d, want 0", len(resp.Dependencies))
	}
}

func TestHandleRoot_DetailFormat_NilHealthDetails(t *testing.T) {
	// HealthDetails returns nil before Start().
	mock := &mockHealthChecker{
		details: nil,
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]json.RawMessage
	_ = json.NewDecoder(rec.Body).Decode(&raw)
	if _, ok := raw["dependencies"]; !ok {
		t.Error("missing dependencies field")
	}
}

func TestHandleRoot_DetailFormat_UnknownState(t *testing.T) {
	// Before first check: Healthy=nil, Status=unknown.
	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{
			"svc:h:80": {
				Healthy: nil,
				Status:  dephealth.StatusUnknown,
				Detail:  "unknown",
				Type:    dephealth.TypeHTTP,
				Name:    "svc",
				Host:    "h",
				Port:    "80",
				Labels:  map[string]string{},
			},
		},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp DetailStatusResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	dep := resp.Dependencies["svc:h:80"]
	if dep.Healthy != nil {
		t.Errorf("Healthy = %v, want nil (unknown)", dep.Healthy)
	}
	if dep.Status != dephealth.StatusUnknown {
		t.Errorf("Status = %q, want %q", dep.Status, dephealth.StatusUnknown)
	}
}

// --- Auth Integration Tests ---

func newTestServerWithAuth(mock *mockHealthChecker, authCfg config.AuthConfig) *Server {
	return New(mock, "test-app", 5*time.Second, authCfg)
}

func defaultMock() *mockHealthChecker {
	return &mockHealthChecker{
		health: map[string]bool{"svc:host:80": true},
	}
}

func TestAuth_None_AllEndpointsOpen(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{})

	for _, path := range []string{"/", "/healthz", "/readyz", "/metrics"} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", path, rr.Code, http.StatusOK)
		}
	}
}

func TestAuth_BearerOnStatus(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{
		Status: &config.ZoneAuth{
			Method: "bearer",
			Token:  "secret-token",
		},
	})

	// / without token → 401.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/ no auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// / with correct token → 200.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/ with token: status = %d, want %d", rr.Code, http.StatusOK)
	}

	// /metrics should be open.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("/metrics: status = %d, want %d", rr.Code, http.StatusOK)
	}

	// /healthz should be open.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("/healthz: status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuth_BasicOnMetrics(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{
		Metrics: &config.ZoneAuth{
			Method:   "basic",
			Username: "admin",
			Password: "secret",
		},
	})

	// /metrics without auth → 401.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/metrics no auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// /metrics with correct basic auth → 200.
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.SetBasicAuth("admin", "secret")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/metrics with auth: status = %d, want %d", rr.Code, http.StatusOK)
	}

	// / should be open.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("/: status = %d, want %d", rr.Code, http.StatusOK)
	}

	// /readyz should be open.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("/readyz: status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuth_GlobalBearer(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{
		Method: "bearer",
		Token:  "global-token",
	})

	// / without token → 401.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/ no auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// /metrics without token → 401.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/metrics no auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// Both with token → 200.
	for _, path := range []string{"/", "/metrics"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer global-token")
		rr = httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s with token: status = %d, want %d", path, rr.Code, http.StatusOK)
		}
	}

	// Probes always open.
	for _, path := range []string{"/healthz", "/readyz"} {
		rr = httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", path, rr.Code, http.StatusOK)
		}
	}
}

func TestAuth_GlobalBearerMetricsOverrideNone(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{
		Method: "bearer",
		Token:  "my-token",
		Metrics: &config.ZoneAuth{
			Method: "none",
		},
	})

	// / without token → 401.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/ no auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// / with token → 200.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/ with token: status = %d, want %d", rr.Code, http.StatusOK)
	}

	// /metrics open (override none).
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("/metrics: status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuth_APIKeyOnStatus(t *testing.T) {
	srv := newTestServerWithAuth(defaultMock(), config.AuthConfig{
		Status: &config.ZoneAuth{
			Method: "apikey",
			APIKey: "my-key",
		},
	})

	// / without key → 401.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/ no key: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// / with correct key → 200.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "my-key")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/ with key: status = %d, want %d", rr.Code, http.StatusOK)
	}
}
