package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindConfigRepoRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "fleets"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := findConfigRepoRoot(dir)
	if got != dir {
		t.Fatalf("findConfigRepoRoot = %q want %q", got, dir)
	}
}

func TestActionRootGettingStarted(t *testing.T) {
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "remotr bootstrap") || !strings.Contains(out, "remotr doctor") {
		t.Fatalf("getting started output = %q", out)
	}
}

func TestDoctorNetworkCheckUsesCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	if err := writeServerCertPEM(caPath, srv.Certificate()); err != nil {
		t.Fatal(err)
	}

	check := doctorNetworkCheck(srv.URL, caPath)
	if check.Status != "ok" {
		t.Fatalf("status = %q detail = %q fix = %q", check.Status, check.Detail, check.Fix)
	}
	if !strings.Contains(check.Detail, "/healthz") {
		t.Fatalf("detail = %q", check.Detail)
	}

	noCA := doctorNetworkCheck(srv.URL, "")
	if noCA.Status != "warn" {
		t.Fatalf("expected warn without CA, got %q", noCA.Status)
	}
}

func TestDoctorJSONMissingCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("server_url: https://example.invalid\nstate_dir: "+filepath.Join(dir, "state")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr", "--config", cfg, "doctor", "--json", "--skip-network"}); err == nil {
			t.Fatal("expected doctor to fail")
		}
	})
	if !strings.Contains(out, `"ok": false`) {
		t.Fatalf("doctor json = %q", out)
	}
}

func writeServerCertPEM(path string, cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("missing server certificate")
	}
	block := &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}
	return os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
}
