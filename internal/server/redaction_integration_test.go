package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestSecretCanaryIsAbsentFromLogsSyncAPIAndCLI(t *testing.T) {
	canary := testsupport.SecretCanary("state-report-integration")
	managedPath := filepath.Join(t.TempDir(), "managed.conf")
	artifact := []byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: file\n        name: managed\n        path: " + managedPath + "\n        content: " + canary + "\n")

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	result, err := pipeline.Run(context.Background(), artifact, engine.PolicyReport, nil, nil, "https://remotr.example")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("agent logs leaked canary: %s", logs.String())
	}

	var pending agentsync.Pending
	pending.SetFromPipeline(result.Labels, result.Drift, result.Apply, result.ApplyFailure, "digest-canary")
	syncBody, err := json.Marshal(pending.Request("", "", "v2.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(syncBody), canary) {
		t.Fatalf("Sync payload leaked canary: %s", syncBody)
	}

	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: base\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	caCert, caKey, caPEM := testCAForEnroll(t)
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	tel := &mockTelemetry{stateReports: reg}
	srv := New(Config{
		ConfigRepoPath: repoDir,
		ReleaseRef:     "redaction",
		Registry:       reg,
		Telemetry:      tel,
		Admin:          reg,
		StateReports:   reg,
		CACert:         caCert,
		CAKey:          caKey,
		CACertPEM:      caPEM,
	})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	syncReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(syncBody))
	syncReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	syncRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("Sync status = %d, body = %s", syncRec.Code, syncRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints/"+endpointID+"/state-report", nil)
	apiReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	apiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("API status = %d, body = %s", apiRec.Code, apiRec.Body.String())
	}
	if strings.Contains(apiRec.Body.String(), canary) {
		t.Fatalf("Admin API leaked canary: %s", apiRec.Body.String())
	}

	fixtureDir := t.TempDir()
	fixture, err := json.Marshal(struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}{Status: http.StatusOK, Body: json.RawMessage(apiRec.Body.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "GET_v1_admin_endpoints_"+endpointID+"_state-report.json")
	if err := os.WriteFile(fixturePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "operator")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator.crt", "operator.key", "ca.crt", "state.json"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr",
		"--server-url", "https://demo.remotr.example",
		"--state-dir", stateDir,
		"endpoint", "state", "report", "--endpoint", endpointID, "--json")
	command.Dir = filepath.Clean(filepath.Join("..", ".."))
	command.Env = append(os.Environ(), "REMOTR_DEMO=1", "REMOTR_DEMO_FIXTURES="+fixtureDir)
	cliOutput, _ := command.CombinedOutput() // Drift intentionally returns exit code 4.
	if len(cliOutput) == 0 {
		t.Fatal("operator CLI produced no state report output")
	}
	if strings.Contains(string(cliOutput), canary) {
		t.Fatalf("operator CLI leaked canary: %s", cliOutput)
	}
}
