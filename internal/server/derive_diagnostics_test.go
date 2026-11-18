package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// Baseline adoption names, per endpoint, why no current evidence matched the
// canonical Fleet composition instead of one opaque message.
func TestBaselineAdoptionExplainsEveryRejectedEndpoint(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	artifact := []byte(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
  - name: arch-only
    targetDistros: [Arch]
    resources:
      - kind: file
        name: marker
        path: /tmp/remotr-arch-marker
        content: present
`)
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		t.Fatal(err)
	}
	identities, err := configcompose.EffectiveResources(t.Context(), state, map[string]configcompose.ProviderSelection{
		"base/operators":   {ID: "sudo"},
		"arch-only/marker": {ID: "file"},
	}, "sha256:artifact", nil)
	if err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	for _, id := range identities {
		hashes[id.Address] = id.EffectiveHash
	}
	sudo := registry.StateReportItem{
		Address: "base/operators", Provider: "sudo", ProviderRevision: "sudo-v1", EffectiveHash: hashes["base/operators"],
		Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}
	noHash := sudo
	noHash.EffectiveHash = ""
	noHash.EffectiveHashStatus = "authorization_required"
	at := time.Unix(1, 0)
	for _, ep := range []string{"ep-digest", "ep-nohash", "ep-none", "ep-partial", "ep-release"} {
		if err := reg.RegisterEndpoint(registry.Endpoint{ID: ep, Fleet: "engineering"}); err != nil {
			t.Fatal(err)
		}
	}
	report := func(id, ref, digest string, items ...registry.StateReportItem) {
		reg.SetEndpointStateReport(id, registry.DriftSummary{ReleaseRef: ref, Digest: digest, ReportedAt: at}, registry.StateReportPayload{SchemaVersion: 10, Items: items})
	}
	report("ep-digest", "release-1", "sha256:endpoint-override", sudo)
	report("ep-release", "release-old", "sha256:artifact", sudo)
	report("ep-partial", "release-1", "sha256:artifact", sudo)
	report("ep-nohash", "release-1", "sha256:artifact", noHash)

	srv := New(Config{
		Admin: reg, StateReports: reg, ReleaseRef: "release-1",
		ChangeControl: changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "must-not-create" }}),
		ArtifactStore: derivedPlanArtifactStore{artifact: artifact, digest: "sha256:artifact"},
		CACert:        caCert, CAKey: caKey, CACertPEM: caPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", bytes.NewBufferString(`{}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"no current endpoint evidence matches canonical Fleet composition",
		"ep-digest: report digest sha256:endpoint-override does not match Fleet artifact sha256:artifact",
		"ep-release: report release release-old does not match current release release-1",
		"ep-partial: composed resources not reported: arch-only/marker",
		"ep-nohash: base/operators has no effective hash (authorization_required)",
		"ep-none: no state report",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}
