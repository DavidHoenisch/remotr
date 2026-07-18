package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func TestAdminSecretUploadReturnsOnlyInactiveMetadataAndRefusesPlaintextReadback(t *testing.T) {
	service := testSecretRegistryService(t, nil, nil)
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	auditLog := &mockAuditLog{}
	srv := New(Config{Admin: admin, AuditLog: auditLog, SecretRegistry: service, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	handler := srv.Handler()
	material := []byte("admin-upload-canary")
	uploadURL := "/v1/admin/secrets/versions?name=" + url.QueryEscape("repositories/private") + "&fleet=production"

	unauthenticated := httptest.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(material))
	unauthenticatedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRec, unauthenticated)
	if unauthenticatedRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticatedRec.Code)
	}

	wrongType := httptest.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(material))
	wrongType.Header.Set("Content-Type", "text/plain")
	wrongType.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	wrongTypeRec := httptest.NewRecorder()
	handler.ServeHTTP(wrongTypeRec, wrongType)
	if wrongTypeRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type status = %d", wrongTypeRec.Code)
	}

	empty := httptest.NewRequest(http.MethodPost, uploadURL, http.NoBody)
	empty.Header.Set("Content-Type", "application/octet-stream")
	empty.TLS = wrongType.TLS
	emptyRec := httptest.NewRecorder()
	handler.ServeHTTP(emptyRec, empty)
	if emptyRec.Code != http.StatusBadRequest {
		t.Fatalf("empty upload status = %d", emptyRec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(material))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), material) {
		t.Fatal("upload response exposed plaintext")
	}
	var metadata secrets.VersionMetadata
	if err := json.Unmarshal(rec.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "repositories/private" || metadata.Version != "1" || metadata.Active {
		t.Fatalf("metadata = %#v", metadata)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/secrets?name="+url.QueryEscape("repositories/private"), nil)
	listReq.TLS = req.TLS
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || bytes.Contains(listRec.Body.Bytes(), material) {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	valueReq := httptest.NewRequest(http.MethodGet, "/v1/admin/secrets/value?name="+url.QueryEscape("repositories/private")+"&version=1", nil)
	valueReq.TLS = req.TLS
	valueRec := httptest.NewRecorder()
	handler.ServeHTTP(valueRec, valueReq)
	if valueRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("plaintext read status = %d body=%s", valueRec.Code, valueRec.Body.String())
	}

	if !hasAuditAction(auditLog.events, audit.ActionAdminSecretUpload) || !hasAuditAction(auditLog.events, audit.ActionAdminSecretReadDenied) {
		t.Fatalf("audit events = %#v", auditLog.events)
	}
	encodedAudit, err := json.Marshal(auditLog.events)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedAudit, material) {
		t.Fatal("audit event exposed plaintext")
	}
}

func TestAdminSecretActivationCreatesConnectivityChangeBeforeResolution(t *testing.T) {
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "change-secret-1" }})
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "production", `schemaVersion: 1
configurations:
  - name: office
    resources:
      - kind: networkProfile
        name: wifi
        provider: network-manager
        selector: {name: wlan0}
        profileName: office
        profileType: wifi
        ssid: corp
        credentialRef: remotr:wifi/office@active
`)
	artifactStore := &OnDemandArtifactResolver{RepoRoot: repoDir}
	_, digest, err := resolveFleetDesiredArtifact(t.Context(), artifactStore, repoDir, "production", "release-1")
	if err != nil {
		t.Fatal(err)
	}
	reports := registry.NewMemory()
	if err := reports.RegisterEndpoint(registry.Endpoint{ID: "endpoint-1", Fleet: "production"}); err != nil {
		t.Fatal(err)
	}
	reports.SetEndpointStateReport("endpoint-1", registry.DriftSummary{ReleaseRef: "release-1", Digest: digest, ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{{
		Address: "office/wifi", Provider: "network-profile", ProviderRevision: "networkProfile-v0",
		EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:        registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}}})
	planDeriver := &ChangePlanDeriver{ConfigRepoPath: repoDir, ArtifactStore: artifactStore, StateReports: reports}
	coordinator := NewSecretActivationCoordinator(changes, planDeriver)
	service := testSecretRegistryService(t, coordinator, coordinator)
	planDeriver.Secrets = service
	if _, err := service.Upload(context.Background(), secrets.UploadRequest{Name: "wifi/office", Fleet: "production", Material: []byte("network-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.fleets = []string{"production"}
	admin.endpoints = []registry.Endpoint{{ID: "endpoint-1", Fleet: "production"}}
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	auditLog := &mockAuditLog{}
	srv := New(Config{Admin: admin, AuditLog: auditLog, SecretRegistry: service, Secrets: service, ChangeControl: changes, ConfigRepoPath: repoDir, ArtifactStore: artifactStore, StateReports: reports, ReleaseRef: "release-1", CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	malformed := httptest.NewRequest(http.MethodPost, "/v1/admin/secrets/activate", bytes.NewBufferString(`{"name":"wifi/office","version":"1","material":"must-not-be-accepted"}`))
	malformed.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	malformedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(malformedRec, malformed)
	if malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("malformed activation status = %d body=%s", malformedRec.Code, malformedRec.Body.String())
	}

	staleRevision := httptest.NewRequest(http.MethodPost, "/v1/admin/secrets/activate", bytes.NewBufferString(`{"name":"wifi/office","version":"1"}`))
	staleRevision.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	staleRevisionRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(staleRevisionRec, staleRevision)
	if staleRevisionRec.Code != http.StatusBadRequest || len(changes.List()) != 0 {
		t.Fatalf("stale provider revision status = %d body=%s requests=%#v", staleRevisionRec.Code, staleRevisionRec.Body.String(), changes.List())
	}
	reports.SetEndpointStateReport("endpoint-1", registry.DriftSummary{ReleaseRef: "release-1", Digest: digest, ReportedAt: time.Unix(2, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{{
		Address: "office/wifi", Provider: "network-profile", ProviderRevision: "networkProfile-v1",
		EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:        registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}}})

	body := bytes.NewBufferString(`{"name":"wifi/office","version":"1"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/secrets/activate", body)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activation status = %d body=%s", rec.Code, rec.Body.String())
	}
	requests := changes.List()
	if len(requests) != 1 || requests[0].ID != "change-secret-1" || requests[0].Risk != models.RiskConnectivity || requests[0].ResourceHashes["office/wifi"] == "" {
		t.Fatalf("Change requests = %#v", requests)
	}
	if requests[0].HashContractVersion != effectivehash.SchemaVersion || requests[0].Resources[0].Provider != "network-profile" || requests[0].Resources[0].ProviderRevision != "networkProfile-v1" || effectivehash.Validate(requests[0].Resources[0].DesiredHash) != nil || len(requests[0].Resources[0].PredictedEffects) != 2 || requests[0].Resources[0].PredictedEffects[1].Code != changecontrol.EffectSecretVersionActivate || len(requests[0].FrozenTargets) != 1 || !requests[0].FrozenTargets[0].PreflightReady {
		t.Fatalf("canonical activation plan = %#v", requests[0])
	}
	if !hasAuditAction(auditLog.events, audit.ActionAdminSecretActivate) {
		t.Fatalf("activation audit events = %#v", auditLog.events)
	}
	encodedAudit, err := json.Marshal(auditLog.events)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedAudit, []byte("network-canary")) {
		t.Fatal("activation audit event exposed plaintext")
	}
	resolve := secrets.ResolveRequest{Reference: "remotr:wifi/office@active", Fleet: "production", EndpointID: "endpoint-1", ResourceAddress: "office/wifi", Purpose: "network-credential"}
	if _, err := service.Resolve(context.Background(), resolve); !errors.Is(err, secrets.ErrUnauthorized) {
		t.Fatalf("pending Change resolution err = %v", err)
	}
	if _, err := changes.AuthorizeRollout("change-secret-1", changecontrol.RolloutSpec{ValidFrom: time.Now().Add(-time.Minute), ValidUntil: time.Now().Add(time.Hour)}, "operator-1", "approved network credential rollout"); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), resolve)
	if err != nil || string(resolved.Material) != "network-canary" {
		t.Fatalf("authorized resolution = %#v err=%v", resolved, err)
	}
}

func testSecretRegistryService(t *testing.T, planner secrets.ActivationPlanner, gate secrets.RolloutGate) *secrets.RegistryService {
	t.Helper()
	keyring, err := secrets.NewKeyring("kek-test", map[string][]byte{"kek-test": bytes.Repeat([]byte{0xe1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := secrets.NewRegistryService(secrets.NewMemoryVersionRepository(), envelope, planner, gate)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func hasAuditAction(events []audit.Event, action string) bool {
	for _, event := range events {
		if event.Action == action {
			return true
		}
	}
	return false
}
