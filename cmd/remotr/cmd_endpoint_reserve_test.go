package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/server"
)

type reserveCLIHarness struct {
	t      *testing.T
	cfg    string
	reg    *registry.Memory
	repo   string
	logBuf *strings.Builder
}

func newReserveCLIHarness(t *testing.T) *reserveCLIHarness {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Remotr CLI Test CA"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(der)

	reg := registry.NewMemory()
	op, err := pki.IssueOperatorCredential(caCert, key, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperatorCredential(identity.Fingerprint(op.Cert)); err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "endpoints", "gitlab-runner-01", "manifest.yaml"), "kind: manifest\nmodules:\n  - modules/base.yaml\n")
	writeFile(t, filepath.Join(repo, "modules", "base.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: base
    targetDistros: [Debian]
    targetArch: [x86]
    resources:
      - kind: package
        name: curl
        lifecycle: present
        present: true
`)
	srv := server.New(server.Config{Admin: reg, Enroller: reg, Registry: reg, CACert: caCert, CAKey: key, ConfigRepoPath: repo, ReleaseRef: "rel-abc123"})
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.TLS = &tls.Config{ClientAuth: tls.RequestClientCert, MinVersion: tls.VersionTLS12}
	ts.StartTLS()
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	serverCA := filepath.Join(dir, "server-ca.pem")
	if err := writeServerCertPEM(serverCA, ts.Certificate()); err != nil {
		t.Fatal(err)
	}
	caPEM, _ := os.ReadFile(serverCA)
	if err := opcreds.Save(state, op.OperatorID, string(op.CertPEM), string(op.KeyPEM), string(caPEM)); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.yaml")
	writeFile(t, cfg, "server_url: "+ts.URL+"\nstate_dir: "+state+"\n")
	return &reserveCLIHarness{t: t, cfg: cfg, reg: reg, repo: repo}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// run executes the real remotr command tree and returns stdout, stderr, and the error.
func (h *reserveCLIHarness) run(args ...string) (string, string, error) {
	h.t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		h.t.Fatal(err)
	}
	os.Stderr = w
	var runErr error
	stdout := captureStdout(h.t, func() {
		runErr = newRootCommand().Run(context.Background(), append([]string{"remotr", "--config", h.cfg}, args...))
	})
	_ = w.Close()
	os.Stderr = oldStderr
	stderr, _ := io.ReadAll(r)
	return stdout, string(stderr), runErr
}

// OS-ENR-015, OS-ENR-017, OS-ENR-018 through the operator CLI.
func TestEndpointReserveCLIWorkflow(t *testing.T) {
	h := newReserveCLIHarness(t)

	tokenPath := filepath.Join(t.TempDir(), "enroll.token")
	out, _, err := h.run("endpoint", "reserve", "--fleet", "ubuntu-servers", "--id", "gitlab-runner-01", "--out", tokenPath, "--quiet")
	if err != nil {
		t.Fatalf("reserve: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(tokenPath)
	token := strings.TrimSpace(string(raw))
	if err != nil || token == "" {
		t.Fatalf("token file = %q, %v", raw, err)
	}
	if strings.Contains(out, token) {
		t.Fatal("--quiet reserve printed the bound token")
	}
	for _, want := range []string{"reserved endpoint: gitlab-runner-01", "fleet: ubuntu-servers", "endpoints/gitlab-runner-01/manifest.yaml"} {
		if !strings.Contains(out, want) {
			t.Fatalf("reserve output missing %q:\n%s", want, out)
		}
	}

	table, _, err := h.run("endpoint", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table, "STATE") || !strings.Contains(table, "gitlab-runner-01\tubuntu-servers\treserved") {
		t.Fatalf("endpoint list table:\n%s", table)
	}
	jsonOut, _, err := h.run("endpoint", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var eps []struct {
		ID              string `json:"id"`
		EnrollmentState string `json:"enrollment_state"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &eps); err != nil || len(eps) != 1 || eps[0].EnrollmentState != "reserved" {
		t.Fatalf("endpoint list json = %s (%v)", jsonOut, err)
	}
	if strings.Contains(table+jsonOut, token) {
		t.Fatal("endpoint listing exposed the bound token")
	}

	_, _, err = h.run("endpoint", "unreserve", "gitlab-runner-01")
	if err == nil {
		t.Fatal("unreserve with a released override succeeded without --force")
	}
	for _, want := range []string{"gitlab-runner-01", "rel-abc123", "endpoints/gitlab-runner-01/", "block Git sync", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("unreserve error %q missing %q", err, want)
		}
	}

	forced, stderr, err := h.run("endpoint", "unreserve", "gitlab-runner-01", "--force")
	if err != nil {
		t.Fatalf("forced unreserve: %v", err)
	}
	if !strings.Contains(forced, "unreserved endpoint: gitlab-runner-01") || !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "block Git sync") {
		t.Fatalf("forced unreserve stdout=%q stderr=%q", forced, stderr)
	}
	if _, ok := h.reg.EndpointByID("gitlab-runner-01"); ok {
		t.Fatal("forced unreserve left the reservation")
	}
}

// OS-ENR-003 through the operator CLI: conflicts explain themselves.
func TestEndpointReserveCLIConflictExplains(t *testing.T) {
	h := newReserveCLIHarness(t)
	if _, _, err := h.run("endpoint", "reserve", "--fleet", "ubuntu-servers", "--id", "gitlab-runner-02", "--quiet"); err != nil {
		t.Fatal(err)
	}
	_, _, err := h.run("endpoint", "reserve", "--fleet", "workstations", "--id", "gitlab-runner-02", "--quiet")
	if err == nil || !strings.Contains(err.Error(), "already reserved in another fleet") {
		t.Fatalf("cross-fleet reserve err = %v", err)
	}
}
