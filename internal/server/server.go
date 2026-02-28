// Package server provides the HTTP server for uniproxy.
//
// It exposes four endpoint groups:
//   - Health probes: /healthz (liveness) and /readyz (readiness) — always unauthenticated, always 200 OK.
//   - Status API: GET / — returns dependency health status as JSON. Supports ?detail=true
//     for enriched responses and ?depth=N for recursive HTTP chain fetching.
//   - Metrics: GET /metrics — Prometheus metrics endpoint via promhttp.Handler().
//
// Status and metrics endpoints support independent zone-based authentication
// (Basic Auth, Bearer Token, or API Key).
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/BigKAA/uniproxy/internal/auth"
	"github.com/BigKAA/uniproxy/internal/config"

	"github.com/BigKAA/topologymetrics/sdk-go/dephealth"
)

// HealthChecker is the interface that the dephealth SDK implements.
// It exposes two methods for querying dependency health:
//   - Health() returns a simple map of dependency name to health status (true/false).
//   - HealthDetails() returns a detailed map including host, port, type, latency, etc.
type HealthChecker interface {
	Health() map[string]bool
	HealthDetails() map[string]dephealth.EndpointStatus
}

// StatusResponse is the JSON payload for GET / (simple mode).
// It provides a quick overview of the application identity and dependency health.
type StatusResponse struct {
	Name      string          `json:"name"`      // application name from config
	PodName   string          `json:"podName"`   // Kubernetes pod name from POD_NAME env var
	Namespace string          `json:"namespace"` // Kubernetes namespace from NAMESPACE env var
	Health    map[string]bool `json:"health"`    // dependency name -> healthy (true/false)
}

// DetailStatusResponse is the JSON payload for GET /?detail=true.
// It provides enriched dependency information including host, port, latency,
// and optionally the recursive responses from HTTP dependencies.
type DetailStatusResponse struct {
	Name         string                       `json:"name"`
	PodName      string                       `json:"podName"`
	Namespace    string                       `json:"namespace"`
	Dependencies map[string]*DependencyDetail `json:"dependencies"`
}

// DependencyDetail wraps the SDK's EndpointStatus with an optional recursive
// response from HTTP dependencies. This enables topology chain visualization
// by fetching and embedding downstream dependency statuses.
type DependencyDetail struct {
	dephealth.EndpointStatus
	Response *json.RawMessage `json:"response,omitempty"` // raw JSON from downstream HTTP fetch
}

// MarshalJSON produces a flat JSON object that merges EndpointStatus fields
// with the optional "response" field. This avoids nested JSON where EndpointStatus
// fields would be wrapped in a separate object.
//
// Strategy: marshal EndpointStatus to JSON, then splice in the "response" field
// before the closing brace.
func (d DependencyDetail) MarshalJSON() ([]byte, error) {
	// Marshal the embedded EndpointStatus first.
	base, err := json.Marshal(d.EndpointStatus)
	if err != nil {
		return nil, err
	}
	if d.Response == nil {
		return base, nil
	}
	// Merge: remove closing "}", append ,"response":<value>}
	result := make([]byte, 0, len(base)+len(*d.Response)+15)
	result = append(result, base[:len(base)-1]...) // everything except closing "}"
	result = append(result, `,"response":`...)
	result = append(result, *d.Response...)
	result = append(result, '}')
	return result, nil
}

// Server is the uniproxy HTTP server. It wraps a Chi router configured with
// auth middleware, probe endpoints, status API, and Prometheus metrics handler.
type Server struct {
	router       chi.Router        // Chi router with configured routes and middleware
	dh           HealthChecker     // dephealth SDK instance for querying health status
	name         string            // application name for status responses
	fetchTimeout time.Duration     // timeout for recursive HTTP fetch requests
	authCfg      config.AuthConfig // zone-based auth config for status and metrics endpoints
}

// New creates a new Server wired to the given health checker and configures
// all routes with the appropriate auth middleware.
//
// Parameters:
//   - dh: the dephealth SDK instance that provides health status data.
//   - name: application name included in status responses.
//   - fetchTimeout: maximum time for recursive HTTP fetch in detail mode.
//   - authCfg: zone-based auth configuration for status and metrics endpoints.
func New(dh HealthChecker, name string, fetchTimeout time.Duration, authCfg config.AuthConfig) *Server {
	s := &Server{
		dh:           dh,
		name:         name,
		fetchTimeout: fetchTimeout,
		authCfg:      authCfg,
	}
	s.routes()
	return s
}

// routes sets up the Chi router with all endpoints and middleware.
// Route structure:
//   - GET /healthz — liveness probe (no auth, always 200)
//   - GET /readyz  — readiness probe (no auth, always 200)
//   - GET /        — status API (optional auth via "status" zone)
//   - GET /metrics — Prometheus metrics (optional auth via "metrics" zone)
func (s *Server) routes() {
	r := chi.NewRouter()
	// Recoverer middleware catches panics in handlers and returns 500 instead of crashing.
	r.Use(middleware.Recoverer)

	// Probes — always open, no authentication required.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// Status API — with optional auth middleware for the "status" zone.
	statusAuth := s.authCfg.ResolveZone("status")
	if mw := auth.Middleware(statusAuth); mw != nil {
		r.With(mw).Get("/", s.handleRoot)
	} else {
		r.Get("/", s.handleRoot)
	}

	// Metrics — with optional auth middleware for the "metrics" zone.
	metricsAuth := s.authCfg.ResolveZone("metrics")
	if mw := auth.Middleware(metricsAuth); mw != nil {
		r.With(mw).Handle("/metrics", promhttp.Handler())
	} else {
		r.Handle("/metrics", promhttp.Handler())
	}

	s.router = r
}

// Handler returns the configured http.Handler (the Chi router).
// This is passed to http.Server.Handler in main.go.
func (s *Server) Handler() http.Handler {
	return s.router
}

// handleRoot handles GET / requests. In simple mode, it returns a StatusResponse
// with the application name, pod identity, and a health map (dependency -> bool).
// If ?detail=true is present, it delegates to handleDetail for enriched output.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if detailed mode is requested via query parameter.
	if r.URL.Query().Get("detail") == "true" {
		s.handleDetail(w, r)
		return
	}

	// Simple mode: return application identity + health map.
	resp := StatusResponse{
		Name:      s.name,
		PodName:   os.Getenv("POD_NAME"),
		Namespace: os.Getenv("NAMESPACE"),
		Health:    s.dh.Health(),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode response", "error", err)
	}
}

// handleDetail handles GET /?detail=true requests. It returns a DetailStatusResponse
// with enriched dependency information from the SDK's HealthDetails().
//
// When depth > 0, it performs parallel HTTP GET requests to all HTTP-type dependencies
// to fetch their own status responses, enabling recursive topology chain visualization.
// The depth parameter controls how many levels deep the recursive fetching goes (max 10).
func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	depth := parseDepth(r.URL.Query().Get("depth"))

	// Get detailed health status for all dependencies from the SDK.
	details := s.dh.HealthDetails()
	deps := make(map[string]*DependencyDetail, len(details))
	for key, es := range details {
		deps[key] = &DependencyDetail{
			EndpointStatus: es,
		}
	}

	// Parallel HTTP fetch for HTTP-type dependencies when depth > 0.
	// This enables recursive chain visibility: each HTTP dependency's status endpoint
	// is queried with depth-1, so the response includes downstream dependencies.
	if depth > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), s.fetchTimeout)
		defer cancel()

		var wg sync.WaitGroup
		var mu sync.Mutex // protects concurrent writes to deps map
		for key, dep := range deps {
			// Only fetch from HTTP-type dependencies (other types don't have status endpoints).
			if dep.Type != dephealth.TypeHTTP {
				continue
			}
			wg.Add(1)
			go func(k string, d *DependencyDetail) {
				defer wg.Done()
				result := fetchHTTPResponse(ctx, d.Host, d.Port, depth, s.fetchTimeout)
				mu.Lock()
				deps[k].Response = result
				mu.Unlock()
			}(key, dep)
		}
		wg.Wait()
	}

	resp := DetailStatusResponse{
		Name:         s.name,
		PodName:      os.Getenv("POD_NAME"),
		Namespace:    os.Getenv("NAMESPACE"),
		Dependencies: deps,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode detail response", "error", err)
	}
}

// parseDepth parses the "depth" query parameter for recursive HTTP fetching.
// Returns 1 as default, bounds the value to [0, 10], and treats parse errors as 1.
// Depth 0 means no recursive fetching, depth 10 is the maximum allowed.
func parseDepth(s string) int {
	if s == "" {
		return 1
	}
	d, err := strconv.Atoi(s)
	if err != nil || d < 0 {
		return 1
	}
	if d > 10 {
		return 10
	}
	return d
}

// handleHealthz handles GET /healthz — the Kubernetes liveness probe.
// Always returns 200 OK with "ok" body, indicating the process is alive.
// No authentication is required.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReadyz handles GET /readyz — the Kubernetes readiness probe.
// Always returns 200 OK with "ok" body, indicating the application is ready
// to receive traffic. No authentication is required.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
