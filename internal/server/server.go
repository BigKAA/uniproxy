// Package server provides the HTTP server for uniproxy.
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

// HealthChecker exposes dependency health status.
type HealthChecker interface {
	Health() map[string]bool
	HealthDetails() map[string]dephealth.EndpointStatus
}

// StatusResponse is the JSON payload for GET / (simple mode).
type StatusResponse struct {
	Name      string          `json:"name"`
	PodName   string          `json:"podName"`
	Namespace string          `json:"namespace"`
	Health    map[string]bool `json:"health"`
}

// DetailStatusResponse is the JSON payload for GET /?detail=true.
type DetailStatusResponse struct {
	Name         string                       `json:"name"`
	PodName      string                       `json:"podName"`
	Namespace    string                       `json:"namespace"`
	Dependencies map[string]*DependencyDetail `json:"dependencies"`
}

// DependencyDetail wraps SDK EndpointStatus with an optional recursive response.
type DependencyDetail struct {
	dephealth.EndpointStatus
	Response *json.RawMessage `json:"response,omitempty"`
}

// MarshalJSON produces JSON that merges EndpointStatus fields with the response field.
func (d DependencyDetail) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(d.EndpointStatus)
	if err != nil {
		return nil, err
	}
	if d.Response == nil {
		return base, nil
	}
	// Merge: remove closing }, append ,"response":<value>}
	result := make([]byte, 0, len(base)+len(*d.Response)+15)
	result = append(result, base[:len(base)-1]...)
	result = append(result, `,"response":`...)
	result = append(result, *d.Response...)
	result = append(result, '}')
	return result, nil
}

// Server is the uniproxy HTTP server.
type Server struct {
	router       chi.Router
	dh           HealthChecker
	name         string
	fetchTimeout time.Duration
	authCfg      config.AuthConfig
}

// New creates a new Server wired to the given health checker.
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

func (s *Server) routes() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// Probes — always open.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// Status API — with optional auth middleware.
	statusAuth := s.authCfg.ResolveZone("status")
	if mw := auth.Middleware(statusAuth); mw != nil {
		r.With(mw).Get("/", s.handleRoot)
	} else {
		r.Get("/", s.handleRoot)
	}

	// Metrics — with optional auth middleware.
	metricsAuth := s.authCfg.ResolveZone("metrics")
	if mw := auth.Middleware(metricsAuth); mw != nil {
		r.With(mw).Handle("/metrics", promhttp.Handler())
	} else {
		r.Handle("/metrics", promhttp.Handler())
	}

	s.router = r
}

// Handler returns the http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.URL.Query().Get("detail") == "true" {
		s.handleDetail(w, r)
		return
	}

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

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	depth := parseDepth(r.URL.Query().Get("depth"))

	details := s.dh.HealthDetails()
	deps := make(map[string]*DependencyDetail, len(details))
	for key, es := range details {
		deps[key] = &DependencyDetail{
			EndpointStatus: es,
		}
	}

	// Parallel HTTP fetch for HTTP-type dependencies when depth > 0.
	if depth > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), s.fetchTimeout)
		defer cancel()

		var wg sync.WaitGroup
		var mu sync.Mutex
		for key, dep := range deps {
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

// parseDepth parses the depth query parameter with bounds [0, 10], default 1.
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

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
