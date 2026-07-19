package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSync_unchangedStillReturnsAgentUpgrade(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)

	reg := registry.NewMemory()
	_ = reg.RegisterEndpoint(registry.Endpoint{
		ID:                   "11111111-1111-1111-1111-111111111111",
		Fleet:                "test-fleet",
		DesiredAgentVersion:  "v0.1.12",
		ReportedAgentVersion: "v0.1.11",
	})
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, Admin: reg})

	variants, err := configcompose.RenderFleetVariants(repoDir, "test-fleet")
	if err != nil || len(variants) != 2 {
		t.Fatalf("legacy variants = %d, err=%v", len(variants), err)
	}
	body := []byte(`{"lastDigest":"` + variants[1].Digest + `","agentVersion":"v0.1.11"}`)
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
	if endpointID != "" {
		_, _, digest, _, err := configcompose.RenderEndpoint(repoRoot, endpointID)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	_, _, digest, _, err := configcompose.RenderFleet(repoRoot, fleet)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
