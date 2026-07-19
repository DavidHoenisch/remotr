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
	"time"

	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSyncExistingEndpointCapabilityBlockedRetainsActiveArtifact(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering", LastCheckIn: &registry.CheckInSummary{
		ReleaseRef: "release-active", Digest: "digest-active", At: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v1.2.3", "capabilityDocument": document,
		"lastReleaseRef": "release-active", "lastDigest": "digest-active",
	})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry,
	}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilityBlocked == nil || response.CapabilityBlocked.TargetReleaseRef != "release-target" || len(response.ArtifactYAML) != 0 {
		t.Fatalf("blocked response = %s", rec.Body.String())
	}
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveReleaseRef != "release-active" || state.ActiveDigest != "digest-active" {
		t.Fatalf("retained active state = %+v, ok=%t err=%v", state, ok, err)
	}
	if telemetry.checkInRelease != "" || telemetry.checkInDigest != "" {
		t.Fatalf("unoffered self-report advanced active check-in: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}

func TestSyncNewEndpointCapabilityBlockedIsUnmanaged(t *testing.T) {
	const endpointID = "22222222-2222-2222-2222-222222222222"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilityBlocked == nil || !response.CapabilityBlocked.Unmanaged || len(response.ArtifactYAML) != 0 {
		t.Fatalf("new blocked endpoint response = %s", rec.Body.String())
	}
	if telemetry.checkInRelease != "" || telemetry.checkInDigest != "" {
		t.Fatalf("new blocked endpoint gained active state: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}

func TestSyncUnacknowledgedOfferDoesNotAdvanceActiveArtifact(t *testing.T) {
	const endpointID = "33333333-3333-3333-3333-333333333333"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", "configurations:\n  - name: base\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:command", Revision: "command-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	telemetry := &mockTelemetry{}
	send := func(t *testing.T, lastReleaseRef, lastDigest string) syncResponse {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"agentVersion": "v1.2.3", "capabilityDocument": document,
			"lastReleaseRef": lastReleaseRef, "lastDigest": lastDigest,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-offered", Registry: reg, Telemetry: telemetry}).Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		var response syncResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	first := send(t, "", "")
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok {
		t.Fatalf("offered state ok=%t err=%v", ok, err)
	}
	if state.OfferedReleaseRef != "release-offered" || state.OfferedDigest != first.Digest || state.ActiveDigest != "" {
		t.Fatalf("state after offer = %+v", state)
	}
	if telemetry.checkInDigest != "" {
		t.Fatalf("unacknowledged offer advanced check-in: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}

	second := send(t, "release-offered", first.Digest)
	state, ok, err = reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveReleaseRef != "release-offered" || state.ActiveDigest != first.Digest || state.OfferedDigest != "" {
		t.Fatalf("state after exact acknowledgement = %+v, ok=%t err=%v", state, ok, err)
	}
	if telemetry.checkInRelease != "release-offered" || telemetry.checkInDigest != first.Digest {
		t.Fatalf("exact acknowledgement did not advance check-in: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
	if !second.Unchanged {
		t.Fatalf("acknowledged response = %+v", second)
	}
}

func TestSyncCapabilityBlockedIncludesApprovedAgentUpgrade(t *testing.T) {
	const endpointID = "44444444-4444-4444-4444-444444444444"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{
		ID: endpointID, Fleet: "engineering", ReportedAgentVersion: "v1.2.3", DesiredAgentVersion: "v1.2.4",
	}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "provider:package/apt", Revision: "1"}},
		Facts:        []capabilitydoc.Fact{{Key: "package", Value: "apt"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Admin: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilityBlocked == nil || response.AgentUpgrade == nil || response.AgentUpgrade.Version != "v1.2.4" || len(response.ArtifactYAML) != 0 {
		t.Fatalf("blocked upgrade response = %s", rec.Body.String())
	}
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveDigest != "" || state.CapabilityBlockedTargetRef != "release-target" {
		t.Fatalf("blocked upgrade delivery state = %+v, ok=%t err=%v", state, ok, err)
	}
}

func TestBlockedUpgradeDoesNotInferRuntimeProviderSupport(t *testing.T) {
	endpoint := registry.Endpoint{DesiredAgentVersion: "v1.2.4", ReportedAgentVersion: "v1.2.3"}
	missing := []agentsync.MissingRequirement{{ID: "provider:package/apt", Revision: "1"}}
	if instruction := New(Config{}).compatibleBlockedUpgradeInstruction(endpoint, missing); instruction != nil {
		t.Fatalf("agent upgrade inferred runtime provider support: %+v", instruction)
	}
}
