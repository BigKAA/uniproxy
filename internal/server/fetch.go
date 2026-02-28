package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// fetchHTTPResponse makes an HTTP GET request to an HTTP dependency's detail endpoint
// to fetch its status response for recursive chain visualization.
//
// It constructs the URL as http://<host>:<port>/?detail=true&depth=<depth-1>,
// passing the decremented depth to limit recursion. This enables topology chain
// visibility: uniproxy A -> uniproxy B -> uniproxy C, each level showing its
// own dependencies.
//
// Returns the response body as raw JSON (*json.RawMessage) on success, or nil
// on any error condition:
//   - depth <= 0 (recursion limit reached)
//   - Request creation failure
//   - Network errors (timeout, connection refused, DNS failure)
//   - Non-200 HTTP status codes
//   - Invalid JSON response body
//
// All errors are logged at debug level to avoid log noise from expected failures
// (e.g. dependency is down, not a uniproxy instance, etc.).
func fetchHTTPResponse(ctx context.Context, host, port string, depth int, timeout time.Duration) *json.RawMessage {
	// Base case: recursion limit reached.
	if depth <= 0 {
		return nil
	}

	// Build the detail endpoint URL with decremented depth.
	url := fmt.Sprintf("http://%s:%s/?detail=true&depth=%d", host, port, depth-1)

	// Create a child context with per-request timeout.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("fetch: create request", "url", url, "error", err)
		return nil
	}

	// Execute the HTTP request using the default client.
	resp, err := http.DefaultClient.Do(req)
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

	body, err := io.ReadAll(resp.Body)
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
