// Package auth provides HTTP authentication middleware for uniproxy endpoints.
//
// It supports three authentication methods:
//   - Basic Auth: standard HTTP Basic Authentication with username/password.
//   - Bearer Token: token-based authentication via the Authorization header.
//   - API Key: key-based authentication via the X-API-Key header.
//
// All credential comparisons use constant-time comparison (crypto/subtle)
// to prevent timing side-channel attacks.
//
// Authentication is zone-based: each endpoint zone (status, metrics) can have
// its own auth method. Health probes (/healthz, /readyz) are always unauthenticated.
package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/BigKAA/uniproxy/internal/config"
)

// Middleware returns a Chi-compatible middleware function that enforces authentication
// based on the resolved zone auth configuration.
//
// Returns nil if the auth method is "none" or empty, meaning no middleware is needed
// and the endpoint is publicly accessible.
//
// The method must be validated at config load time, so the default case
// (unsupported method) should never be reached in practice.
func Middleware(za config.ZoneAuth) func(http.Handler) http.Handler {
	switch za.Method {
	case "none", "":
		return nil
	case "basic":
		return basicAuth(za.Username, za.Password)
	case "bearer":
		return bearerAuth(za.Token)
	case "apikey":
		return apiKeyAuth(za.APIKey)
	default:
		// Should not happen — validated at config load time.
		return nil
	}
}

// unauthorized sends an HTTP 401 Unauthorized response with a JSON error body.
// If realm is non-empty, a WWW-Authenticate header is set for Basic Auth,
// prompting the browser to show a login dialog.
func unauthorized(w http.ResponseWriter, realm string) {
	if realm != "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// basicAuth returns HTTP Basic Authentication middleware.
// It extracts credentials from the Authorization header using r.BasicAuth()
// and compares them with the expected username and password using constant-time
// comparison to prevent timing attacks.
func basicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				// Respond with realm="uniproxy" to trigger browser login dialog.
				unauthorized(w, "uniproxy")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerAuth returns Bearer token authentication middleware.
// It expects the Authorization header in the format "Bearer <token>" and uses
// constant-time comparison to prevent timing attacks against the token value.
func bearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			// Validate header format and compare token with constant-time comparison.
			if len(auth) < len(prefix) ||
				auth[:len(prefix)] != prefix ||
				subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(token)) != 1 {
				unauthorized(w, "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// apiKeyAuth returns API Key authentication middleware.
// It reads the key from the X-API-Key request header and uses constant-time
// comparison to prevent timing attacks.
func apiKeyAuth(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
				unauthorized(w, "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
