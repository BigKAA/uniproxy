package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BigKAA/uniproxy/internal/config"
)

// okHandler is a simple handler that returns 200 OK with body "ok".
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

func TestMiddleware_None_ReturnsNil(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "none"})
	if mw != nil {
		t.Error("expected nil middleware for method=none")
	}
}

func TestMiddleware_Empty_ReturnsNil(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: ""})
	if mw != nil {
		t.Error("expected nil middleware for empty method")
	}
}

func TestMiddleware_Unknown_ReturnsNil(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "oauth2"})
	if mw != nil {
		t.Error("expected nil middleware for unknown method")
	}
}

// --- Basic Auth ---

func TestBasicAuth_CorrectCreds(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "basic", Username: "admin", Password: "secret"})
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("admin", "secret")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "ok")
	}
}

func TestBasicAuth_WrongPassword(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "basic", Username: "admin", Password: "secret"})

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("admin", "wrong")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	assertUnauthorizedResponse(t, rr)
	assertWWWAuthenticate(t, rr)
}

func TestBasicAuth_WrongUsername(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "basic", Username: "admin", Password: "secret"})

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("other", "secret")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestBasicAuth_NoHeader(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "basic", Username: "admin", Password: "secret"})

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	assertWWWAuthenticate(t, rr)
}

// --- Bearer Auth ---

func TestBearerAuth_CorrectToken(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "bearer", Token: "my-token"})
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBearerAuth_WrongToken(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "bearer", Token: "my-token"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	assertUnauthorizedResponse(t, rr)
}

func TestBearerAuth_NoHeader(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "bearer", Token: "my-token"})

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestBearerAuth_MissingPrefix(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "bearer", Token: "my-token"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "my-token")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestBearerAuth_BearerWithoutSpace(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "bearer", Token: "my-token"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearermy-token")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// --- API Key Auth ---

func TestAPIKeyAuth_CorrectKey(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "apikey", APIKey: "secret-key"})
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "secret-key")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIKeyAuth_WrongKey(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "apikey", APIKey: "secret-key"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	assertUnauthorizedResponse(t, rr)
}

func TestAPIKeyAuth_NoHeader(t *testing.T) {
	mw := Middleware(config.ZoneAuth{Method: "apikey", APIKey: "secret-key"})

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// --- Response format helpers ---

func assertUnauthorizedResponse(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("body error = %q, want %q", body["error"], "unauthorized")
	}
}

func assertWWWAuthenticate(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	header := rr.Header().Get("WWW-Authenticate")
	if header == "" {
		t.Error("expected WWW-Authenticate header, got empty")
	}
	expected := `Basic realm="uniproxy"`
	if header != expected {
		t.Errorf("WWW-Authenticate = %q, want %q", header, expected)
	}
}
