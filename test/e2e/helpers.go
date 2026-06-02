//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

const (
	defaultBaseURL            = "https://localhost:8443"
	defaultBootstrapTokenFile = "compose/runtime/bootstrap.token"
)

func baseURL() string {
	return envOr("REMOTR_E2E_URL", defaultBaseURL)
}

func certsDir() string {
	return envOr("REMOTR_E2E_CERTS", defaultCertsDir())
}

func defaultCAPath() string {
	return filepath.Join(certsDir(), "ca.crt")
}

func defaultCertsDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("compose", "runtime", "certs")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "compose", "runtime", "certs"))
}

func defaultBootstrapTokenPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return defaultBootstrapTokenFile
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", defaultBootstrapTokenFile))
}

func bootstrapTokenPath() string {
	return envOr("REMOTR_E2E_BOOTSTRAP_TOKEN_FILE", defaultBootstrapTokenPath())
}

func waitBootstrapToken(t *testing.T, timeout time.Duration) string {
	t.Helper()
	path := bootstrapTokenPath()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if token := readBootstrapTokenFile(path); token != "" {
			return token
		}
		if token := readBootstrapTokenDocker(); token != "" {
			return token
		}
		time.Sleep(500 * time.Millisecond)
	}
	return ""
}

func readBootstrapTokenFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readBootstrapTokenDocker() string {
	container := envOr("REMOTR_E2E_SERVER_CONTAINER", "compose-remotr-server-1")
	out, err := exec.Command("docker", "exec", container, "cat", "/var/lib/remotr/bootstrap.token").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse ca pem")
	}
	return pool, nil
}

// serverTLSClient returns an HTTP client that trusts the Remotr CA but does not
// present a client certificate (server TLS only).
func serverTLSClient(caPath string) (*http.Client, error) {
	pool, err := loadCAPool(caPath)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		},
	}, nil
}

func freshEnrollToken(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("REMOTR_E2E_ENROLL_TOKEN")); v != "" {
		return v
	}

	ctx := context.Background()
	dbURL := envOr("REMOTR_E2E_DATABASE_URL", "postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable")
	st, err := pgstore.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("create enroll token: connect database: %v", err)
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(time.Hour)
	if _, err := st.CreateEnrollmentToken(ctx, token, "test-fleet", expires); err != nil {
		t.Fatalf("create enroll token: %v", err)
	}
	return token
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func composeRuntimeDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("compose", "runtime")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "compose", "runtime"))
}

func agentStateDir(name string) string {
	return filepath.Join(envOr("REMOTR_E2E_RUNTIME", composeRuntimeDir()), "agent-"+name)
}

func waitForAgentEnrolled(t *testing.T, name string) {
	t.Helper()
	dir := agentStateDir(name)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := enrolledAgentTLS(name); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("agent %q did not finish enrollment (expected %s/state.json)", name, dir)
}

func enrolledAgentTLS(name string) (*tls.Config, string, error) {
	dir := agentStateDir(name)
	if !credentials.Present(dir) {
		return nil, "", fmt.Errorf("agent not enrolled at %s", dir)
	}
	layout, err := credentials.Layout(dir)
	if err != nil {
		return nil, "", err
	}
	st, err := credentials.LoadState(dir)
	if err != nil {
		return nil, "", err
	}
	tlsCfg, err := tlsconfig.ClientTLSConfig(layout.Cert, layout.Key, layout.CA)
	if err != nil {
		return nil, "", err
	}
	return tlsCfg, st.EndpointID, nil
}
