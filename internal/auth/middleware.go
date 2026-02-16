package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/BigKAA/uniproxy/internal/config"
)

// Middleware returns a Chi middleware that enforces authentication.
// If zone auth method is "none", returns nil (no middleware needed).
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
		// Should not happen — validated at config load.
		return nil
	}
}

func unauthorized(w http.ResponseWriter, realm string) {
	if realm != "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

func basicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				unauthorized(w, "uniproxy")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
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
