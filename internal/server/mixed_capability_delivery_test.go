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
	"testing"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestMixedSchemaCapabilityDeliveryFixtures(t *testing.T) {
	repoDir := t.TempDir()
	writeMixedDesiredFixture(t, repoDir, "command", "testdata/mixed-command-desired.yaml")
	writeMixedDesiredFixture(t, repoDir, "package", "testdata/mixed-package-desired.yaml")

	reg := registry.NewMemory()
	endpoints := map[string]string{
		"legacy":    "10000000-0000-0000-0000-000000000001",
		"modern":    "20000000-0000-0000-0000-000000000002",
		"facts":     "30000000-0000-0000-0000-000000000003",
		"downgrade": "40000000-0000-0000-0000-000000000004",
		"recovery":  "50000000-0000-0000-0000-000000000005",
	}
	for name, endpointID := range endpoints {
		fleet := "command"
		if name == "recovery" {
			fleet = "package"
		}
		if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: fleet}); err != nil {
			t.Fatal(err)
		}
	}
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-mixed", Registry: reg})

	t.Run("known legacy receives lossless schema 0", func(t *testing.T) {
		response := sendMixedSync(t, server, endpoints["legacy"], map[string]any{"agentVersion": "v0.1.12"})
		if len(response.ArtifactYAML) == 0 || bytes.Contains(response.ArtifactYAML, []byte("schemaVersion:")) || response.CapabilityBlocked != nil {
			t.Fatalf("legacy response = %+v artifact=%s", response, response.ArtifactYAML)
		}
	})

	t.Run("modern current document receives schema 1", func(t *testing.T) {
		document := mixedCommandDocument(t, "x86")
		response := sendMixedSync(t, server, endpoints["modern"], map[string]any{
			"agentVersion": document.AgentVersion, "capabilityDocument": document,
		})
		if !bytes.Contains(response.ArtifactYAML, []byte("schemaVersion: 1")) || response.CapabilityBlocked != nil {
			t.Fatalf("modern response = %+v artifact=%s", response, response.ArtifactYAML)
		}
	})

	t.Run("current fact changes replace current selection evidence", func(t *testing.T) {
		first := mixedCommandDocument(t, "x86")
		second := mixedCommandDocument(t, "arm")
		sendMixedSync(t, server, endpoints["facts"], map[string]any{
			"agentVersion": first.AgentVersion, "capabilityDocument": first,
		})
		response := sendMixedSync(t, server, endpoints["facts"], map[string]any{
			"agentVersion": second.AgentVersion, "capabilityDocument": second,
		})
		if !bytes.Contains(response.ArtifactYAML, []byte("schemaVersion: 1")) {
			t.Fatalf("fact-change response = %+v artifact=%s", response, response.ArtifactYAML)
		}
		current, ok := server.currentCapabilityEvidence(endpoints["facts"])
		if !ok || current.Digest != second.Digest || current.Digest == first.Digest {
			t.Fatalf("current fact evidence = %+v, ok=%t", current, ok)
		}
		persisted, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpoints["facts"])
		if err != nil || !ok || persisted.Digest != second.Digest {
			t.Fatalf("persisted fact evidence = %+v, ok=%t err=%v", persisted, ok, err)
		}
	})

	t.Run("known agent downgrade offers schema 0 without advancing active schema", func(t *testing.T) {
		document := mixedCommandDocument(t, "x86")
		first := sendMixedSync(t, server, endpoints["downgrade"], map[string]any{
			"agentVersion": document.AgentVersion, "capabilityDocument": document,
		})
		acknowledged := sendMixedSync(t, server, endpoints["downgrade"], map[string]any{
			"agentVersion": document.AgentVersion, "capabilityDocument": document,
			"lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		})
		if !acknowledged.Unchanged {
			t.Fatalf("modern acknowledgement = %+v", acknowledged)
		}
		downgraded := sendMixedSync(t, server, endpoints["downgrade"], map[string]any{"agentVersion": "v0.1.12"})
		if len(downgraded.ArtifactYAML) == 0 || bytes.Contains(downgraded.ArtifactYAML, []byte("schemaVersion:")) || downgraded.CapabilityBlocked != nil {
			t.Fatalf("downgrade response = %+v artifact=%s", downgraded, downgraded.ArtifactYAML)
		}
		state, ok, err := reg.GetEndpointDeliveryState(t.Context(), endpoints["downgrade"])
		if err != nil || !ok || state.ActiveSchemaVersion != 1 || state.OfferedSchemaVersion != 0 || state.ActiveDigest != first.Digest || state.OfferedDigest != downgraded.Digest {
			t.Fatalf("downgrade delivery state = %+v, ok=%t err=%v", state, ok, err)
		}
		if _, ok := server.currentCapabilityEvidence(endpoints["downgrade"]); ok {
			t.Fatal("downgrade reused stale modern current evidence")
		}
	})

	t.Run("missing revision blocks then current reconnect evidence recovers", func(t *testing.T) {
		missing := mixedPackageDocument(t, "0")
		blocked := sendMixedSync(t, server, endpoints["recovery"], map[string]any{
			"agentVersion": missing.AgentVersion, "capabilityDocument": missing,
		})
		if blocked.CapabilityBlocked == nil || len(blocked.ArtifactYAML) != 0 || !hasMixedMissingRequirement(blocked, "provider:package/apt", "1") {
			t.Fatalf("missing-revision response = %+v", blocked)
		}

		recovered := mixedPackageDocument(t, "1")
		response := sendMixedSync(t, server, endpoints["recovery"], map[string]any{
			"agentVersion": recovered.AgentVersion, "capabilityDocument": recovered,
		})
		if response.CapabilityBlocked != nil || !bytes.Contains(response.ArtifactYAML, []byte("schemaVersion: 1")) {
			t.Fatalf("recovery response = %+v artifact=%s", response, response.ArtifactYAML)
		}
		persisted, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpoints["recovery"])
		if err != nil || !ok || persisted.Digest != recovered.Digest || persisted.Digest == missing.Digest {
			t.Fatalf("recovered evidence = %+v, ok=%t err=%v", persisted, ok, err)
		}
	})
}

func writeMixedDesiredFixture(t *testing.T, repoDir, fleet, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFleetDesired(t, repoDir, fleet, string(raw))
}

func sendMixedSync(t *testing.T, server *Server, endpointID string, body any) syncResponse {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	request := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func mixedCommandDocument(t *testing.T, architecture string) capabilitydoc.Document {
	t.Helper()
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:command", Revision: "command-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: architecture}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func mixedPackageDocument(t *testing.T, providerRevision string) capabilitydoc.Document {
	t.Helper()
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{
			{ID: "resource:package", Revision: "package-v1"},
			{ID: "provider:package/apt", Revision: providerRevision},
		},
		Facts: []capabilitydoc.Fact{{Key: "package", Value: "apt"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func hasMixedMissingRequirement(response syncResponse, id, revision string) bool {
	if response.CapabilityBlocked == nil {
		return false
	}
	for _, requirement := range response.CapabilityBlocked.MissingRequirements {
		if requirement.ID == id && requirement.Revision == revision {
			return true
		}
	}
	return false
}
