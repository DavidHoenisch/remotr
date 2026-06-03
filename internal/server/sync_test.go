package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSync_returnsFleetArtifactForAuthenticatedEndpoint(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := []byte("configurations:\n  - name: smoke\n    description: e2e\n")
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), artifact, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{
		ID:    "11111111-1111-1111-1111-111111111111",
		Fleet: "test-fleet",
	})

	uri, err := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		ConfigRepoPath: repoDir,
		ReleaseRef:     "e2e",
		Registry:       reg,
	})

	body, _ := json.Marshal(map[string]string{"lastDigest": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}},
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Unchanged {
		t.Fatal("expected artifact, got unchanged")
	}
	if string(resp.ArtifactYAML) != string(artifact) {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
	if resp.ReleaseRef != "e2e" {
		t.Fatalf("releaseRef = %q", resp.ReleaseRef)
	}
	if resp.RemediationPolicy != "auto" {
		t.Fatalf("remediationPolicy = %q", resp.RemediationPolicy)
	}
}

func TestSync_returnsEndpointOverrideWhenPresent(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()

	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations:\n  - name: fleet\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	epDir := filepath.Join(repoDir, "endpoints", endpointID)
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := []byte("configurations:\n  - name: override\n")
	if err := os.WriteFile(filepath.Join(epDir, "desired.yaml"), override, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})

	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "e2e", Registry: reg})

	body, _ := json.Marshal(map[string]string{"lastDigest": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.ArtifactYAML) != string(override) {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
}

func TestSync_includesFleetRemediationPolicy(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "lab")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	reg.SetRemediationPolicy("lab", "report")
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "lab"})

	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{
		ConfigRepoPath: repoDir,
		Registry:       reg,
		FleetSettings:  reg,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"lastDigest":""}`)))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.RemediationPolicy != "report" {
		t.Fatalf("remediationPolicy = %q", resp.RemediationPolicy)
	}
}

func TestSync_gzipWhenAcceptEncoding(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "test-fleet"})
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg})

	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"lastDigest":""}`)))
	req.Header.Set("Accept-Encoding", "gzip")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("encoding = %q", rec.Header().Get("Content-Encoding"))
	}

	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	var resp syncResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Digest == "" {
		t.Fatal("expected digest in gzip response")
	}
}

type mockTelemetry struct {
	labels       map[string]string
	driftDigest  string
	driftJSON    []byte
	applyAddress string
}

func (m *mockTelemetry) RecordEndpointCheckIn(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockTelemetry) UpsertEndpointLabels(_ context.Context, _ string, labels map[string]string) error {
	m.labels = labels
	return nil
}

func (m *mockTelemetry) InsertDriftReport(_ context.Context, _, _, digest string, reportJSON []byte) error {
	m.driftDigest = digest
	m.driftJSON = reportJSON
	return nil
}

func (m *mockTelemetry) InsertApplyFailure(_ context.Context, _, _, resourceAddress, _ string) error {
	m.applyAddress = resourceAddress
	return nil
}

func (m *mockTelemetry) UpdateAgentUpgradeReport(context.Context, string, string, string, string, bool) error {
	return nil
}

func TestSync_persistsTelemetry(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "test-fleet"})
	tel := &mockTelemetry{}
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, Telemetry: tel})

	body := []byte(`{
		"lastDigest":"abc",
		"labels":{"site":"berlin"},
		"drift":{"digest":"d1","report":{"drifted":true}},
		"applyFailure":{"resourceAddress":"cfg/res","message":"failed"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if tel.labels["site"] != "berlin" {
		t.Fatalf("labels = %+v", tel.labels)
	}
	if tel.driftDigest != "d1" {
		t.Fatalf("drift digest = %q", tel.driftDigest)
	}
	if tel.applyAddress != "cfg/res" {
		t.Fatalf("apply address = %q", tel.applyAddress)
	}
}

func TestSync_rejectsRequestWithoutEndpointIdentity(t *testing.T) {
	srv := New(Config{Registry: registry.NewMemory(), ConfigRepoPath: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
