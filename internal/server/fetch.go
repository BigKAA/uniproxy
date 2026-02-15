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

// fetchHTTPResponse makes an HTTP GET request to the dependency's detail endpoint
// and returns the response body as raw JSON. It passes depth-1 to support recursive
// chain visibility. Returns nil on any error (timeout, connection refused, non-200,
// invalid JSON).
func fetchHTTPResponse(ctx context.Context, host, port string, depth int, timeout time.Duration) *json.RawMessage {
	if depth <= 0 {
		return nil
	}

	url := fmt.Sprintf("http://%s:%s/?detail=true&depth=%d", host, port, depth-1)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("fetch: create request", "url", url, "error", err)
		return nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("fetch: request failed", "url", url, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("fetch: non-200 status", "url", url, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug("fetch: read body", "url", url, "error", err)
		return nil
	}

	// Validate that the response is valid JSON.
	if !json.Valid(body) {
		slog.Debug("fetch: invalid JSON", "url", url)
		return nil
	}

	raw := json.RawMessage(body)
	return &raw
}
