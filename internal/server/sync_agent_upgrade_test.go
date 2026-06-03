package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSync_unchangedStillReturnsAgentUpgrade(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "configurations: []\n"
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	_ = reg.RegisterEndpoint(registry.Endpoint{
		ID:                  "11111111-1111-1111-1111-111111111111",
		Fleet:               "test-fleet",
		DesiredAgentVersion: "v0.1.12",
		ReportedAgentVersion: "v0.1.11",
	})
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, Admin: reg})

	body := []byte(`{"lastDigest":"` + mustDigest(t, repoDir, "test-fleet", "") + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Unchanged {
		t.Fatal("expected unchanged")
	}
	if resp.AgentUpgrade == nil || resp.AgentUpgrade.Version != "v0.1.12" {
		t.Fatalf("agentUpgrade = %+v", resp.AgentUpgrade)
	}
}

func mustDigest(t *testing.T, repoRoot, fleet, endpointID string) string {
	t.Helper()
	_, d, err := configrepo.ResolveArtifact(repoRoot, fleet, endpointID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
