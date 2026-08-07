package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
)

type fleetSettingsMutationStub struct {
	server *Server
	failed bool
}

type countingFleetSettingsMutator struct{ calls int }

func (m *countingFleetSettingsMutator) SetRemediationPolicy(context.Context, string, string) error {
	m.calls++
	return nil
}

func TestRedisBarrierOutageRejectsPublicMutationBeforePersistence(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "55555555-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	mutator := &countingFleetSettingsMutator{}
	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM, FleetSettingsMutator: mutator, FastPath: FastPathConfig{Enabled: true, Backend: FastPathRedis, RedisURL: "redis://:secret@127.0.0.1:1", RedisPrefix: "outage", ServingProcesses: 2}})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/fleets/engineering/remediation-policy", bytes.NewBufferString(`{"policy":"report"}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if mutator.calls != 0 {
		t.Fatalf("durable mutator calls = %d, want 0", mutator.calls)
	}
}

func (s *fleetSettingsMutationStub) SetRemediationPolicy(_ context.Context, fleet, policy string) error {
	if _, present := s.server.fastPath.entries["target"]; present {
		panic("target fleet cache entry survived mutation begin")
	}
	if _, present := s.server.fastPath.entries["unrelated"]; !present {
		panic("unrelated fleet cache entry was evicted")
	}
	if snapshot := s.server.fastPath.authoritySnapshot("target", fleet); snapshot.stable {
		panic("target fleet authority remained stable during mutation")
	}
	if s.failed {
		return errors.New("injected policy persistence failure")
	}
	return nil
}

// OS-USF-006: the authenticated policy mutation uses fleet-scoped
// invalidation and leaves unrelated fleet decisions intact.
func TestAdminFleetRemediationPolicyInvalidatesOnlyTargetFleet(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "44444444-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	mutator := &fleetSettingsMutationStub{}
	srv := New(Config{
		Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
		FleetSettingsMutator: mutator,
		FastPath:             FastPathConfig{Enabled: true, ServingProcesses: 1},
	})
	mutator.server = srv
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	}
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(request.documentHashes.Documents)},
	}
	for endpoint, fleet := range map[string]string{"target": "engineering", "unrelated": "finance"} {
		snapshot := srv.fastPath.authoritySnapshot(endpoint, fleet)
		srv.fastPath.putWithSnapshot(endpoint, fleet, "fingerprint", request, response, time.Unix(0, 0), snapshot)
	}

	req := httptest.NewRequest(http.MethodPut, "/v1/admin/fleets/engineering/remediation-policy", bytes.NewBufferString(`{"policy":"report"}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, present := srv.fastPath.entries["target"]; present {
		t.Fatal("target entry returned after mutation")
	}
	if _, present := srv.fastPath.entries["unrelated"]; !present {
		t.Fatal("unrelated entry missing after mutation")
	}

	snapshot := srv.fastPath.authoritySnapshot("target", "engineering")
	srv.fastPath.putWithSnapshot("target", "engineering", "fingerprint", request, response, time.Unix(1, 0), snapshot)
	mutator.failed = true
	failedReq := httptest.NewRequest(http.MethodPut, "/v1/admin/fleets/engineering/remediation-policy", bytes.NewBufferString(`{"policy":"auto"}`))
	failedReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	failedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(failedRec, failedReq)
	if failedRec.Code != http.StatusInternalServerError {
		t.Fatalf("failed mutation status = %d body = %s", failedRec.Code, failedRec.Body.String())
	}
	if _, present := srv.fastPath.entries["target"]; present {
		t.Fatal("failed mutation restored target cache entry")
	}
	if _, present := srv.fastPath.entries["unrelated"]; !present {
		t.Fatal("failed mutation evicted unrelated fleet entry")
	}
}
