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

func TestSyncBlockedEndpointTelemetryRemainsAttributedToActiveDigest(t *testing.T) {
	const endpointID = "55555555-5555-5555-5555-555555555555"
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
		"drift": map[string]any{
			"digest": "digest-active",
			"report": map[string]any{"schemaVersion": 2, "inCompliance": false, "items": []any{}},
		},
	})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry,
	}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("capabilityBlocked")) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if telemetry.driftRelease != "release-active" || telemetry.driftDigest != "digest-active" || len(telemetry.driftJSON) == 0 {
		t.Fatalf("blocked telemetry identity = %q/%q payload=%s, want active artifact", telemetry.driftRelease, telemetry.driftDigest, telemetry.driftJSON)
	}
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.TargetReleaseRef != "release-target" || state.ActiveReleaseRef != "release-active" || state.ActiveDigest != "digest-active" {
		t.Fatalf("delivery state = %+v, ok=%t err=%v", state, ok, err)
	}
}

func TestSyncCapabilityBlockedPreservesBoundedPendingTelemetry(t *testing.T) {
	const endpointID = "66666666-6666-6666-6666-666666666666"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", "configurations:\n  - name: base\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering", LastCheckIn: &registry.CheckInSummary{
		ReleaseRef: "release-active", Digest: "digest-active", At: time.Now().UTC(),
	}}); err != nil {
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
	document.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v1.2.3", "capabilityDocument": document,
		"lastReleaseRef": "release-active", "lastDigest": "digest-active",
		"labels":    map[string]string{"site": "berlin"},
		"usernames": []string{"alice"},
		"systemInfo": map[string]any{
			"digest": "system-digest", "report": map[string]any{"hostname": "endpoint-six"},
		},
		"drift": map[string]any{
			"digest": "digest-active",
			"report": map[string]any{"schemaVersion": 2, "inCompliance": false, "items": []any{}},
		},
		"applyFailure": map[string]any{"resourceAddress": "base/command", "message": "failed"},
	})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry,
	}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("capabilityBlocked")) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if telemetry.labels["site"] != "berlin" || len(telemetry.usernames) != 1 || telemetry.systemDigest != "system-digest" || telemetry.applyAddress != "base/command" {
		t.Fatalf("blocked pending telemetry was lost: %+v", telemetry)
	}
	if telemetry.driftRelease != "release-active" || telemetry.driftDigest != "digest-active" || len(telemetry.driftJSON) == 0 {
		t.Fatalf("blocked state report identity = %q/%q payload=%s", telemetry.driftRelease, telemetry.driftDigest, telemetry.driftJSON)
	}
	if telemetry.checkInRelease != "" || telemetry.checkInDigest != "" {
		t.Fatalf("blocked response advanced check-in: %q/%q", telemetry.checkInRelease, telemetry.checkInDigest)
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

// OS-AEC-109. Public seam: an authenticated capability-blocked endpoint can
// escape through the current explicitly approved, integrity-controlled agent
// release instead of deadlocking on its old runtime evidence.
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
		ID: endpointID, Fleet: "engineering", ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "v0.6.32",
	}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v0.5.1",
		Capabilities: []capabilitydoc.Capability{{ID: "provider:package/apt", Revision: "1"}},
		Facts: []capabilitydoc.Fact{
			{Key: "architecture", Value: "x86"}, {Key: "package", Value: "apt"},
		},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"agentVersion": "v0.5.1", "capabilityDocument": document})
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
	if response.CapabilityBlocked == nil || response.AgentUpgrade == nil || response.AgentUpgrade.Version != "v0.6.32" || len(response.ArtifactYAML) != 0 {
		t.Fatalf("blocked upgrade response = %s", rec.Body.String())
	}
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveDigest != "" || state.CapabilityBlockedTargetRef != "release-target" {
		t.Fatalf("blocked upgrade delivery state = %+v, ok=%t err=%v", state, ok, err)
	}
}

func TestBlockedUpgradeDoesNotInferRuntimeProviderSupport(t *testing.T) {
	endpoint := registry.Endpoint{DesiredAgentVersion: "v0.6.8", ReportedAgentVersion: "v0.5.1"}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v0.5.1",
		Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if instruction := New(Config{}).compatibleBlockedUpgradeInstruction(endpoint, &document); instruction == nil {
		t.Fatal("missing runtime provider support incorrectly suppressed an eligible upgrade")
	}
}

func TestUpgradedEndpointIsReevaluatedBeforeExactArtifactAcknowledgement(t *testing.T) {
	const endpointID = "77777777-7777-7777-7777-777777777777"
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
		ID: endpointID, Fleet: "engineering", ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "v0.6.8",
		LastCheckIn: &registry.CheckInSummary{ReleaseRef: "release-active", Digest: "digest-active", At: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Admin: reg})
	send := func(t *testing.T, version string, capabilities []capabilitydoc.Capability, lastRelease, lastDigest string) syncResponse {
		t.Helper()
		document, err := (capabilitydoc.Document{
			DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: version,
			Capabilities: capabilities,
			Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}, {Key: "package", Value: "apt"}},
		}).WithCanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{
			"agentVersion": version, "capabilityDocument": document,
			"lastReleaseRef": lastRelease, "lastDigest": lastDigest,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response syncResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	old := send(t, "v0.5.1", []capabilitydoc.Capability{{ID: "provider:package/apt", Revision: "1"}}, "release-active", "digest-active")
	if old.CapabilityBlocked == nil || old.AgentUpgrade == nil || len(old.ArtifactYAML) != 0 {
		t.Fatalf("legacy blocked response = %+v", old)
	}
	if err := reg.UpdateAgentUpgradeReport(t.Context(), endpointID, "v0.6.8", "completed", "", true); err != nil {
		t.Fatal(err)
	}
	stillBlocked := send(t, "v0.6.8", []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}}, "release-active", "digest-active")
	if stillBlocked.CapabilityBlocked == nil || stillBlocked.AgentUpgrade != nil || len(stillBlocked.ArtifactYAML) != 0 {
		t.Fatalf("post-upgrade runtime evidence was inferred: %+v", stillBlocked)
	}
	state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveDigest != "digest-active" {
		t.Fatalf("blocked active state = %+v ok=%t err=%v", state, ok, err)
	}

	actual := []capabilitydoc.Capability{
		{ID: "resource:package", Revision: "package-v1"},
		{ID: "provider:package/apt", Revision: "1"},
	}
	offer := send(t, "v0.6.8", actual, "release-active", "digest-active")
	if offer.CapabilityBlocked != nil || len(offer.ArtifactYAML) == 0 || offer.Digest == "" {
		t.Fatalf("compatible offer = %+v", offer)
	}
	state, ok, err = reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveDigest != "digest-active" || state.OfferedDigest != offer.Digest {
		t.Fatalf("unacknowledged offer state = %+v ok=%t err=%v", state, ok, err)
	}

	ack := send(t, "v0.6.8", actual, "release-target", offer.Digest)
	if !ack.Unchanged {
		t.Fatalf("exact acknowledgement response = %+v", ack)
	}
	state, ok, err = reg.GetEndpointDeliveryState(t.Context(), endpointID)
	if err != nil || !ok || state.ActiveDigest != offer.Digest || state.ActiveReleaseRef != "release-target" || state.OfferedDigest != "" {
		t.Fatalf("acknowledged state = %+v ok=%t err=%v", state, ok, err)
	}
}
