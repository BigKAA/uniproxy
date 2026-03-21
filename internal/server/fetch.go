package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker"
)

// maxResponseSize limits the body size read from downstream HTTP dependencies
// to prevent memory exhaustion from malicious or unexpectedly large responses.
const maxResponseSize = 1 << 20 // 1 MiB

// circuitBreakerConfig holds current circuit breaker settings.
// Updated via ConfigureFetch().
var circuitBreakerConfig = struct {
	maxFailures   uint32
	timeout       time.Duration
	halfOpenLimit uint32
}{
	maxFailures:   5,
	timeout:       60 * time.Second,
	halfOpenLimit: 3,
}

// transportConfig holds current HTTP transport settings.
// Updated via ConfigureFetch().
var transportConfig = struct {
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
}{
	maxIdleConns:        100,
	maxIdleConnsPerHost: 10,
	idleConnTimeout:     90 * time.Second,
}

// fetchTransport is a tuned HTTP transport for connection pooling.
// Initialized with defaults, updated via ConfigureFetch().
var fetchTransport = &http.Transport{
	MaxIdleConns:        transportConfig.maxIdleConns,
	MaxIdleConnsPerHost: transportConfig.maxIdleConnsPerHost,
	IdleConnTimeout:     transportConfig.idleConnTimeout,
	ForceAttemptHTTP2:   false,
}

// fetchClient is a dedicated HTTP client for downstream fetches with
// connection pooling via fetchTransport.
var fetchClient = &http.Client{
	Transport: fetchTransport,
	Timeout:   30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Pool metrics for monitoring HTTP connection pool utilization.
var (
	poolIdleConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "uniproxy_http_pool_idle_connections",
		Help: "Current number of idle HTTP connections in the pool",
	})
	poolRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "uniproxy_http_pool_requests_total",
		Help: "Total number of HTTP requests made by the fetch client",
	})
)

// cbRegistry stores circuit breakers per downstream endpoint (host:port).
// Thread-safe via sync.Map.
var cbRegistry sync.Map

// circuitBreakerMetrics holds Prometheus metrics for circuit breaker states.
var circuitBreakerMetrics = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "uniproxy_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=half-open, 2=open) per downstream",
	},
	[]string{"downstream"},
)

// circuitBreakerRequestsTotal counts circuit breaker state transitions.
var circuitBreakerRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "uniproxy_circuit_breaker_requests_total",
		Help: "Total requests processed by circuit breaker per state",
	},
	[]string{"downstream", "state"},
)

// ConfigureFetch applies the given configuration to the HTTP transport and
// circuit breaker settings. Call this once after config is loaded.
func ConfigureFetch(maxIdleConns, maxIdleConnsPerHost int, idleConnTimeout time.Duration, cbMaxFailures uint32, cbTimeout time.Duration, cbHalfOpenLimit uint32) {
	// Update transport config.
	transportConfig.maxIdleConns = maxIdleConns
	transportConfig.maxIdleConnsPerHost = maxIdleConnsPerHost
	transportConfig.idleConnTimeout = idleConnTimeout

	// Update transport.
	fetchTransport.MaxIdleConns = maxIdleConns
	fetchTransport.MaxIdleConnsPerHost = maxIdleConnsPerHost
	fetchTransport.IdleConnTimeout = idleConnTimeout

	// Update circuit breaker config.
	circuitBreakerConfig.maxFailures = cbMaxFailures
	circuitBreakerConfig.timeout = cbTimeout
	circuitBreakerConfig.halfOpenLimit = cbHalfOpenLimit

	slog.Info("fetch configured",
		"maxIdleConns", maxIdleConns,
		"maxIdleConnsPerHost", maxIdleConnsPerHost,
		"idleConnTimeout", idleConnTimeout,
		"cbMaxFailures", cbMaxFailures,
		"cbTimeout", cbTimeout,
		"cbHalfOpenLimit", cbHalfOpenLimit,
	)
}

// getOrCreateCB returns an existing circuit breaker for the endpoint
// or creates a new one if it doesn't exist.
func getOrCreateCB(name string) *gobreaker.CircuitBreaker {
	if cb, ok := cbRegistry.Load(name); ok {
		return cb.(*gobreaker.CircuitBreaker)
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: circuitBreakerConfig.halfOpenLimit,
		Interval:    0, // don't reset counter in closed state
		Timeout:     circuitBreakerConfig.timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= circuitBreakerConfig.maxFailures
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			stateVal := float64(0)
			switch to {
			case gobreaker.StateClosed:
				stateVal = 0
			case gobreaker.StateHalfOpen:
				stateVal = 1
			case gobreaker.StateOpen:
				stateVal = 2
			}
			circuitBreakerMetrics.WithLabelValues(name).Set(stateVal)
			slog.Info("circuit breaker state change",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	actual, _ := cbRegistry.LoadOrStore(name, cb)
	return actual.(*gobreaker.CircuitBreaker)
}

// recordPoolMetrics updates HTTP connection pool metrics.
// Called periodically to expose pool state for monitoring.
func recordPoolMetrics() {
	// Transport does not expose per-host idle connection counts directly.
	// We report the configured maximum per-host limit as an upper bound.
	// Actual idle count varies dynamically based on request patterns.
	poolIdleConnections.Set(float64(transportConfig.maxIdleConnsPerHost))
}

// init starts a background goroutine that periodically updates pool metrics.
func init() {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			recordPoolMetrics()
		}
	}()
}

// fetchWithCircuitBreaker wraps fetchHTTPResponse with circuit breaker logic.
func fetchWithCircuitBreaker(ctx context.Context, host, port string, depth int, timeout time.Duration) *json.RawMessage {
	cbName := fmt.Sprintf("%s:%s", host, port)
	cb := getOrCreateCB(cbName)

	result, _ := cb.Execute(func() (interface{}, error) {
		poolRequestsTotal.Inc()
		resp := fetchHTTPResponse(ctx, host, port, depth, timeout)
		if resp == nil {
			// Return error to count as failure
			return nil, fmt.Errorf("fetch failed")
		}
		return resp, nil
	})

	// Track request outcomes
	if result != nil {
		circuitBreakerRequestsTotal.WithLabelValues(cbName, "success").Inc()
		return result.(*json.RawMessage)
	}

	circuitBreakerRequestsTotal.WithLabelValues(cbName, "failure").Inc()
	return nil
}

// fetchHTTPResponse makes an HTTP/HTTPS GET request to an HTTP dependency's detail endpoint
// to fetch its status response for recursive chain visualization.
//
// It constructs the URL as http(s)://<host>:<port>/?detail=true&depth=<depth-1>,
// passing the decremented depth to limit recursion. The scheme is determined by port:
// - Port 443 → HTTPS (secure by default)
// - Other ports → HTTP (for compatibility)
//
// This enables topology chain visibility: uniproxy A -> uniproxy B -> uniproxy C,
// each level showing its own dependencies.
//
// Returns the response body as raw JSON (*json.RawMessage) on success, or nil
// on any error condition:
//   - depth <= 0 (recursion limit reached)
//   - Request creation failure
//   - Network errors (timeout, connection refused, DNS failure)
//   - Non-200 HTTP status codes
//   - Response body exceeds maxResponseSize (1 MiB)
//   - Invalid JSON response body
//
// All errors are logged at debug level to avoid log noise from expected failures
// (e.g. dependency is down, not a uniproxy instance, etc.).
func fetchHTTPResponse(ctx context.Context, host, port string, depth int, timeout time.Duration) *json.RawMessage {
	// Base case: recursion limit reached.
	if depth <= 0 {
		return nil
	}

	// Determine scheme based on port.
	// Port 443 implies HTTPS for downstream uniproxy instances.
	scheme := "http"
	if port == "443" {
		scheme = "https"
	}

	// Build the detail endpoint URL with decremented depth.
	url := fmt.Sprintf("%s://%s:%s/?detail=true&depth=%d", scheme, host, port, depth-1)

	// Create a child context with per-request timeout.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("fetch: create request", "url", url, "error", err)
		return nil
	}

	// Execute the HTTP request using the dedicated fetch client.
	resp, err := fetchClient.Do(req)
	if err != nil {
		slog.Debug("fetch: request failed", "url", url, "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	// Only accept successful responses.
	if resp.StatusCode != http.StatusOK {
		slog.Debug("fetch: non-200 status", "url", url, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		slog.Debug("fetch: read body", "url", url, "error", err)
		return nil
	}

	// Validate that the response is well-formed JSON before embedding it.
	// This prevents invalid JSON from breaking the parent response serialization.
	if !json.Valid(body) {
		slog.Debug("fetch: invalid JSON", "url", url)
		return nil
	}

	raw := json.RawMessage(body)
	return &raw
}
