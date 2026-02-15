package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
)

func TestFetchHTTPResponse_Success(t *testing.T) {
	// Simulated uniproxy downstream returning detail response.
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":         "downstream",
			"podName":      "pod-1",
			"namespace":    "ns",
			"dependencies": map[string]any{},
		})
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)
	result := fetchHTTPResponse(context.Background(), host, port, 1, 5*time.Second)
	if result == nil {
		t.Fatal("expected non-nil response")
	}

	var resp map[string]any
	if err := json.Unmarshal(*result, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["name"] != "downstream" {
		t.Errorf("name = %v, want downstream", resp["name"])
	}
}

func TestFetchHTTPResponse_DepthPropagation(t *testing.T) {
	// Verify that depth-1 is passed in the query string.
	var receivedDepth string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedDepth = r.URL.Query().Get("depth")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"ds"}`))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)

	// depth=3 should pass depth=2 to downstream.
	fetchHTTPResponse(context.Background(), host, port, 3, 5*time.Second)
	if receivedDepth != "2" {
		t.Errorf("received depth = %q, want %q", receivedDepth, "2")
	}

	// depth=1 should pass depth=0 to downstream.
	fetchHTTPResponse(context.Background(), host, port, 1, 5*time.Second)
	if receivedDepth != "0" {
		t.Errorf("received depth = %q, want %q", receivedDepth, "0")
	}
}

func TestFetchHTTPResponse_Timeout(t *testing.T) {
	// Slow server should result in nil response.
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{"name":"slow"}`))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)
	result := fetchHTTPResponse(context.Background(), host, port, 1, 100*time.Millisecond)
	if result != nil {
		t.Error("expected nil response for timeout")
	}
}

func TestFetchHTTPResponse_UnreachableHost(t *testing.T) {
	// Non-routable address should return nil.
	result := fetchHTTPResponse(context.Background(), "192.0.2.1", "1", 1, 500*time.Millisecond)
	if result != nil {
		t.Error("expected nil response for unreachable host")
	}
}

func TestFetchHTTPResponse_NonJSONResponse(t *testing.T) {
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not json"))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)
	result := fetchHTTPResponse(context.Background(), host, port, 1, 5*time.Second)
	if result != nil {
		t.Error("expected nil response for non-JSON body")
	}
}

func TestFetchHTTPResponse_Non200Status(t *testing.T) {
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"fail"}`))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)
	result := fetchHTTPResponse(context.Background(), host, port, 1, 5*time.Second)
	if result != nil {
		t.Error("expected nil response for non-200 status")
	}
}

func TestFetchHTTPResponse_DepthZero(t *testing.T) {
	called := false
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"name":"ds"}`))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)
	result := fetchHTTPResponse(context.Background(), host, port, 0, 5*time.Second)
	if result != nil {
		t.Error("expected nil response for depth=0")
	}
	if called {
		t.Error("server should not have been called with depth=0")
	}
}

// TestHandleDetail_WithHTTPFetch tests the full integration: detail handler
// fetches HTTP dependency responses when depth > 0.
func TestHandleDetail_WithHTTPFetch(t *testing.T) {
	// Start a downstream server simulating another uniproxy instance.
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":         "backend",
			"podName":      "pod-b",
			"namespace":    "ns",
			"dependencies": map[string]any{},
		})
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)

	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{
			fmt.Sprintf("backend:%s:%s", host, port): {
				Healthy:  boolPtr(true),
				Status:   dephealth.StatusOK,
				Detail:   "200_ok",
				Latency:  10 * time.Millisecond,
				Type:     dephealth.TypeHTTP,
				Name:     "backend",
				Host:     host,
				Port:     port,
				Critical: true,
				Labels:   map[string]string{},
			},
			"db:pg.svc:5432": {
				Healthy:  boolPtr(true),
				Status:   dephealth.StatusOK,
				Detail:   "ok",
				Type:     dephealth.TypePostgres,
				Name:     "db",
				Host:     "pg.svc",
				Port:     "5432",
				Critical: true,
				Labels:   map[string]string{},
			},
		},
	}
	srv := newTestServer(mock)

	// depth=1 (default) — HTTP dep should have response.
	req := httptest.NewRequest(http.MethodGet, "/?detail=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// Use raw JSON to verify response field, because EndpointStatus.UnmarshalJSON
	// is promoted to DependencyDetail and swallows the "response" key during Decode.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode top-level: %v", err)
	}

	var deps map[string]json.RawMessage
	if err := json.Unmarshal(raw["dependencies"], &deps); err != nil {
		t.Fatalf("decode dependencies: %v", err)
	}

	httpKey := fmt.Sprintf("backend:%s:%s", host, port)
	httpDepJSON, ok := deps[httpKey]
	if !ok {
		t.Fatalf("missing dependency %s", httpKey)
	}

	var httpFields map[string]json.RawMessage
	if err := json.Unmarshal(httpDepJSON, &httpFields); err != nil {
		t.Fatalf("decode http dep: %v", err)
	}

	respField, ok := httpFields["response"]
	if !ok || string(respField) == "null" {
		t.Fatal("HTTP dep response should not be nil/absent with depth=1")
	}

	// Verify the fetched response content.
	var fetched map[string]any
	if err := json.Unmarshal(respField, &fetched); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if fetched["name"] != "backend" {
		t.Errorf("fetched name = %v, want backend", fetched["name"])
	}

	// Postgres dep should NOT have response field.
	pgDepJSON, ok := deps["db:pg.svc:5432"]
	if !ok {
		t.Fatal("missing dependency db:pg.svc:5432")
	}
	var pgFields map[string]json.RawMessage
	json.Unmarshal(pgDepJSON, &pgFields)
	if _, hasResp := pgFields["response"]; hasResp {
		t.Error("non-HTTP dep should not have response field")
	}
}

// TestHandleDetail_DepthZero_NoFetch ensures depth=0 produces no fetch.
func TestHandleDetail_DepthZero_NoFetch(t *testing.T) {
	called := false
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"name":"ds"}`))
	}))
	defer downstream.Close()

	host, port := hostPort(t, downstream)

	mock := &mockHealthChecker{
		details: map[string]dephealth.EndpointStatus{
			fmt.Sprintf("svc:%s:%s", host, port): {
				Healthy: boolPtr(true),
				Status:  dephealth.StatusOK,
				Type:    dephealth.TypeHTTP,
				Name:    "svc",
				Host:    host,
				Port:    port,
				Labels:  map[string]string{},
			},
		},
	}
	srv := newTestServer(mock)

	req := httptest.NewRequest(http.MethodGet, "/?detail=true&depth=0", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if called {
		t.Error("downstream should not have been called with depth=0")
	}

	var raw map[string]json.RawMessage
	json.NewDecoder(rec.Body).Decode(&raw)
	var deps map[string]json.RawMessage
	json.Unmarshal(raw["dependencies"], &deps)

	key := fmt.Sprintf("svc:%s:%s", host, port)
	depJSON, ok := deps[key]
	if !ok {
		t.Fatalf("missing dependency %s", key)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(depJSON, &fields)
	if _, hasResp := fields["response"]; hasResp {
		t.Error("response field should be absent with depth=0")
	}
}

// hostPort extracts host and port from an httptest.Server URL.
func hostPort(t *testing.T, s *httptest.Server) (string, string) {
	t.Helper()
	// URL format: http://127.0.0.1:PORT
	u := s.URL
	// Skip "http://"
	addr := u[len("http://"):]
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	t.Fatalf("cannot parse host:port from %s", u)
	return "", ""
}
