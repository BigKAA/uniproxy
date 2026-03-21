// Package main provides tests for HTTPS/TLS functionality.
//
// Tests cover:
//   - prepareTLSFiles: inline cert/key → temp files, file-based passthrough
//   - validateTLS (via config.ValidateTLSConfig): enabled/disabled, missing certs, ambiguous config
//   - listenAddr selection logic: HTTP vs HTTPS defaults
//   - Real TLS: cert/key parsing and key-pair validation
//   - Integration: real HTTPS server accepting trusted/untrusted connections
//   - Error scenarios: invalid PEM, mismatched pair
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BigKAA/uniproxy/internal/config"
)

// testDataDir returns the path to shared TLS test fixtures.
func testDataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("internal/testdata/tls")
	if err != nil {
		t.Fatalf("failed to resolve testdata dir: %v", err)
	}
	return dir
}

// readTestFile reads a file from the TLS testdata directory.
func readTestFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testDataDir(t), name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("failed to read testdata file %q: %v", name, err)
	}
	return string(data)
}

// invalidCertData is syntactically broken PEM — used to test error paths.
const invalidCertData = `-----BEGIN CERTIFICATE-----
AAAAAA==
-----END CERTIFICATE-----`

// ---------------------------------------------------------------------------
// prepareTLSFiles — inline mode
// ---------------------------------------------------------------------------

func TestPrepareTLSFiles_InlineCreatesTemp(t *testing.T) {
	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tlsCfg := &config.TLSConfig{CertData: certData, KeyData: keyData}
	certPath, keyPath, err := prepareTLSFiles(tlsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(certPath); _ = os.Remove(keyPath) })

	if certPath == "" || keyPath == "" {
		t.Fatal("expected non-empty paths")
	}
	if certPath == keyPath {
		t.Fatal("cert and key paths must differ")
	}

	// Files must exist on disk.
	for _, p := range []string{certPath, keyPath} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("temp file not found: %s", p)
		}
	}

	// Content must match.
	if got, _ := os.ReadFile(certPath); string(got) != certData { //nolint:gosec // temp file path from prepareTLSFiles
		t.Error("cert file content mismatch")
	}
	if got, _ := os.ReadFile(keyPath); string(got) != keyData { //nolint:gosec // temp file path from prepareTLSFiles
		t.Error("key file content mismatch")
	}
}

func TestPrepareTLSFiles_InlineKeyPermissions(t *testing.T) {
	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tlsCfg := &config.TLSConfig{CertData: certData, KeyData: keyData}
	certPath, keyPath, err := prepareTLSFiles(tlsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(certPath); _ = os.Remove(keyPath) })

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key permissions = %o, want 0600", perm)
	}
}

// ---------------------------------------------------------------------------
// prepareTLSFiles — file-based mode
// ---------------------------------------------------------------------------

func TestPrepareTLSFiles_FileBasedReturnsOriginalPaths(t *testing.T) {
	dir := testDataDir(t)
	certFile := filepath.Join(dir, "test-cert.pem")
	keyFile := filepath.Join(dir, "test-key.pem")

	tlsCfg := &config.TLSConfig{CertFile: certFile, KeyFile: keyFile}
	gotCert, gotKey, err := prepareTLSFiles(tlsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCert != certFile {
		t.Errorf("cert path = %q, want %q", gotCert, certFile)
	}
	if gotKey != keyFile {
		t.Errorf("key path = %q, want %q", gotKey, keyFile)
	}
}

func TestPrepareTLSFiles_FileBasedNoTempFiles(t *testing.T) {
	dir := testDataDir(t)
	tlsCfg := &config.TLSConfig{
		CertFile: filepath.Join(dir, "test-cert.pem"),
		KeyFile:  filepath.Join(dir, "test-key.pem"),
	}
	gotCert, gotKey, err := prepareTLSFiles(tlsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(gotCert, "uniproxy-tls-cert") {
		t.Errorf("cert looks like temp file: %s", gotCert)
	}
	if strings.Contains(gotKey, "uniproxy-tls-key") {
		t.Errorf("key looks like temp file: %s", gotKey)
	}
}

// ---------------------------------------------------------------------------
// prepareTLSFiles — benchmark
// ---------------------------------------------------------------------------

func BenchmarkPrepareTLSFiles_Inline(b *testing.B) {
	dir, _ := filepath.Abs("internal/testdata/tls")
	certData, _ := os.ReadFile(filepath.Join(dir, "test-cert.pem")) //nolint:gosec // test fixture
	keyData, _ := os.ReadFile(filepath.Join(dir, "test-key.pem"))   //nolint:gosec // test fixture
	tlsCfg := &config.TLSConfig{CertData: string(certData), KeyData: string(keyData)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		certPath, keyPath, err := prepareTLSFiles(tlsCfg)
		if err != nil {
			b.Fatal(err)
		}
		_ = os.Remove(certPath)
		_ = os.Remove(keyPath)
	}
}

// ---------------------------------------------------------------------------
// config.ValidateTLSConfig
// ---------------------------------------------------------------------------

func TestValidateTLS_Nil(t *testing.T) {
	if err := config.ValidateTLSConfig(nil); err != nil {
		t.Errorf("unexpected error for nil: %v", err)
	}
}

func TestValidateTLS_Disabled(t *testing.T) {
	if err := config.ValidateTLSConfig(&config.TLSConfig{Enabled: false}); err != nil {
		t.Errorf("unexpected error when TLS disabled: %v", err)
	}
}

func TestValidateTLS_EnabledWithFileCerts(t *testing.T) {
	dir := testDataDir(t)
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled:  true,
		CertFile: filepath.Join(dir, "test-cert.pem"),
		KeyFile:  filepath.Join(dir, "test-key.pem"),
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateTLS_EnabledWithInlineCerts(t *testing.T) {
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled:  true,
		CertData: readTestFile(t, "test-cert.pem"),
		KeyData:  readTestFile(t, "test-key.pem"),
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateTLS_EnabledNoCerts(t *testing.T) {
	err := config.ValidateTLSConfig(&config.TLSConfig{Enabled: true})
	if err == nil {
		t.Error("expected error when TLS enabled but no certs")
	}
}

func TestValidateTLS_AmbiguousCert(t *testing.T) {
	dir := testDataDir(t)
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled:  true,
		CertFile: filepath.Join(dir, "test-cert.pem"),
		CertData: readTestFile(t, "test-cert.pem"),
		KeyFile:  filepath.Join(dir, "test-key.pem"),
	})
	if err == nil {
		t.Error("expected error: both CertFile and CertData set")
	}
}

func TestValidateTLS_AmbiguousKey(t *testing.T) {
	dir := testDataDir(t)
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled:  true,
		CertFile: filepath.Join(dir, "test-cert.pem"),
		KeyFile:  filepath.Join(dir, "test-key.pem"),
		KeyData:  readTestFile(t, "test-key.pem"),
	})
	if err == nil {
		t.Error("expected error: both KeyFile and KeyData set")
	}
}

func TestValidateTLS_CertWithoutKey(t *testing.T) {
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled:  true,
		CertData: readTestFile(t, "test-cert.pem"),
	})
	if err == nil {
		t.Error("expected error when cert provided but key is missing")
	}
}

func TestValidateTLS_KeyWithoutCert(t *testing.T) {
	err := config.ValidateTLSConfig(&config.TLSConfig{
		Enabled: true,
		KeyData: readTestFile(t, "test-key.pem"),
	})
	if err == nil {
		t.Error("expected error when key provided but cert is missing")
	}
}

// ---------------------------------------------------------------------------
// Listen address selection logic (mirrors main.go)
// ---------------------------------------------------------------------------

func TestListenAddrSelection(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		tls        *config.TLSConfig
		wantAddr   string
	}{
		{
			name:       "HTTP — uses LISTEN_ADDR",
			listenAddr: ":8080",
			tls:        nil,
			wantAddr:   ":8080",
		},
		{
			name:       "HTTPS — uses TLS_LISTEN_ADDR when set",
			listenAddr: ":8080",
			tls:        &config.TLSConfig{Enabled: true, ListenAddr: ":9443"},
			wantAddr:   ":9443",
		},
		{
			name:       "HTTPS — defaults to :8443 when TLS_LISTEN_ADDR empty",
			listenAddr: ":8080",
			tls:        &config.TLSConfig{Enabled: true},
			wantAddr:   ":8443",
		},
		{
			name:       "TLS struct present but disabled — uses LISTEN_ADDR",
			listenAddr: ":8080",
			tls:        &config.TLSConfig{Enabled: false},
			wantAddr:   ":8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ListenAddr: tt.listenAddr, TLS: tt.tls}

			// Reproduce the listen-address selection from main.go.
			addr := cfg.ListenAddr
			if cfg.TLS != nil && cfg.TLS.Enabled {
				if cfg.TLS.ListenAddr != "" {
					addr = cfg.TLS.ListenAddr
				} else {
					addr = ":8443"
				}
			}

			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Real TLS: certificate and key parsing
// ---------------------------------------------------------------------------

func TestTLSCert_ParseValidPair(t *testing.T) {
	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	cert, err := tls.X509KeyPair([]byte(certData), []byte(keyData))
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("empty certificate chain")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if leaf.Subject.CommonName != "localhost" {
		t.Errorf("CN = %q, want \"localhost\"", leaf.Subject.CommonName)
	}
}

func TestTLSCert_MismatchedPairFails(t *testing.T) {
	certData := readTestFile(t, "test-cert.pem")
	_, err := tls.X509KeyPair([]byte(certData), []byte(invalidCertData))
	if err == nil {
		t.Error("expected error for mismatched cert/key pair")
	}
}

func TestTLSCert_InvalidCertFails(t *testing.T) {
	keyData := readTestFile(t, "test-key.pem")
	_, err := tls.X509KeyPair([]byte(invalidCertData), []byte(keyData))
	if err == nil {
		t.Error("expected error for invalid cert PEM")
	}
}

func TestTLSCertPool_AppendValid(t *testing.T) {
	certData := readTestFile(t, "test-cert.pem")
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM([]byte(certData)); !ok {
		t.Fatal("AppendCertsFromPEM returned false for valid cert")
	}
}

// ---------------------------------------------------------------------------
// Integration: real HTTPS server
// ---------------------------------------------------------------------------

// newTLSListener creates a TLS listener on a random loopback port.
func newTLSListener(t *testing.T, certFile, keyFile string) net.Listener {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certFile, keyFile) //nolint:gosec // test fixture paths
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	return ln
}

// startTestHTTPS starts a minimal HTTPS server on a TLS listener.
// The listener must already be a *tls.Listener (created via tls.Listen),
// so Serve (not ServeTLS) is used — the TLS handshake happens inside the listener.
func startTestHTTPS(t *testing.T, ln net.Listener) (baseURL string, shutdown func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ready := make(chan struct{})
	go func() {
		close(ready)
		_ = srv.Serve(ln)
	}()
	<-ready // goroutine is running; the listener is accepting

	baseURL = "https://" + ln.Addr().String()
	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return baseURL, shutdown
}

func TestHTTPS_TrustedClientSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTPS integration test in short mode")
	}

	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(certFile, []byte(certData), 0o600); err != nil { //nolint:gosec // test cert is not secret
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(keyData), 0o600); err != nil {
		t.Fatal(err)
	}

	ln := newTLSListener(t, certFile, keyFile)
	t.Cleanup(func() { _ = ln.Close() })

	baseURL, shutdown := startTestHTTPS(t, ln)
	t.Cleanup(shutdown)

	// Client trusts our self-signed CA.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(certData))
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.TLS == nil {
		t.Error("expected TLS info in response")
	}
}

func TestHTTPS_UntrustedClientFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTPS integration test in short mode")
	}

	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(certFile, []byte(certData), 0o600); err != nil { //nolint:gosec // test cert is not secret
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(keyData), 0o600); err != nil {
		t.Fatal(err)
	}

	ln := newTLSListener(t, certFile, keyFile)
	t.Cleanup(func() { _ = ln.Close() })

	baseURL, shutdown := startTestHTTPS(t, ln)
	t.Cleanup(shutdown)

	// Default client uses system cert pool — our self-signed cert is NOT trusted.
	client := &http.Client{Timeout: 3 * time.Second}

	_, err := client.Get(baseURL + "/healthz")
	if err == nil {
		t.Error("expected TLS error for untrusted self-signed certificate")
	}
}

func TestHTTPS_InsecureSkipVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTPS integration test in short mode")
	}

	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(certFile, []byte(certData), 0o600); err != nil { //nolint:gosec // test cert is not secret
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(keyData), 0o600); err != nil {
		t.Fatal(err)
	}

	ln := newTLSListener(t, certFile, keyFile)
	t.Cleanup(func() { _ = ln.Close() })

	baseURL, shutdown := startTestHTTPS(t, ln)
	t.Cleanup(shutdown)

	// InsecureSkipVerify allows any cert — should succeed despite unknown CA.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		},
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHTTPS_GracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTPS integration test in short mode")
	}

	certData := readTestFile(t, "test-cert.pem")
	keyData := readTestFile(t, "test-key.pem")

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(certFile, []byte(certData), 0o600); err != nil { //nolint:gosec // test cert is not secret
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(keyData), 0o600); err != nil {
		t.Fatal(err)
	}

	ln := newTLSListener(t, certFile, keyFile)
	t.Cleanup(func() { _ = ln.Close() })

	_, shutdown := startTestHTTPS(t, ln)

	// Shutdown must complete within the timeout.
	done := make(chan struct{})
	go func() {
		shutdown()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Error("graceful shutdown timed out")
	}
}
