package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// OS-AEC-076: operator-visible Change-control evidence and authorization
// survive reconstruction of the server process from the same durable store.
func TestAdminChangeControlSurvivesServerRestart(t *testing.T) {
	const canary = "change-control-admin-secret-canary"
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	store := &restartChangeControlStore{}
	ids := []string{"request-1", "rollout-1"}
	index := 0
	changes, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			id := ids[index]
			index++
			return id
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	serverBeforeRestart := New(withDerivedSudoPlan(t, Config{Admin: admin, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM}))
	request := func(srv *Server, method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	unsafeAdoption := []byte(`{"release_ref":"release-1","artifact_digest":"sha256:artifact","targets":[{"endpoint_id":"endpoint-a","compatible":true,"preflight_ready":true}],"resources":[{"address":"base/firewall","desired_hash":"sha256:firewall","risk":"connectivity","provider":"nftables","predicted_effects":["` + canary + `"]}]}`)
	if rec := request(serverBeforeRestart, http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", unsafeAdoption); rec.Code != http.StatusBadRequest {
		t.Fatalf("unclassified effect: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(store.payload, []byte(canary)) {
		t.Fatalf("unclassified effect reached durable state: %s", store.payload)
	}

	if rec := request(serverBeforeRestart, http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", []byte(`{}`)); rec.Code != http.StatusOK {
		t.Fatalf("create before restart: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(serverBeforeRestart, http.MethodPost, "/v1/admin/change-requests/request-1/authorize", []byte(`{"attempt_limit":2,"max_concurrency":1,"justification":"CHG-42"}`)); rec.Code != http.StatusOK {
		t.Fatalf("authorize before restart: status=%d body=%s", rec.Code, rec.Body.String())
	}

	restored, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	serverAfterRestart := New(Config{Admin: admin, ChangeControl: restored, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	rec := request(serverAfterRestart, http.MethodGet, "/v1/admin/change-requests/request-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("show after restart: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got changecontrol.ChangeRequest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "request-1" || got.ReleaseRef != "release-1" || effectivehash.Validate(got.ResourceHashes["base/operators"]) != nil {
		t.Fatalf("restored frozen plan = %+v", got)
	}
	if len(got.Resources) != 1 || len(got.Resources[0].PredictedEffects) != 1 || got.Resources[0].PredictedEffects[0].Code != changecontrol.EffectSudoPolicyReplace {
		t.Fatalf("restored classified effects = %+v", got.Resources)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(canary)) || bytes.Contains(store.payload, []byte(canary)) {
		t.Fatalf("unclassified canary survived restart: body=%s state=%s", rec.Body.String(), store.payload)
	}
	if got.AuthorizationState != changecontrol.AuthorizationActive || len(got.Approvals) != 1 || got.Approvals[0].OperatorID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("restored authorization = %+v", got)
	}
	if len(got.AuditHistory) != 3 || got.AuditHistory[0].Action != changecontrol.AuditCreated || got.AuditHistory[1].Action != changecontrol.AuditBaselineAdoption || got.AuditHistory[2].Action != changecontrol.AuditRolloutAuthorized {
		t.Fatalf("restored audit history = %+v", got.AuditHistory)
	}
}

// OS-AEC-077: an unexpired lease continues to occupy its concurrency slot
// after restart, and attempt accounting remains authoritative after expiry.
func TestSyncChangeControlLeaseAndAttemptsSurviveServerRestart(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}

	const endpointA = "11111111-1111-1111-1111-111111111111"
	const endpointB = "33333333-3333-3333-3333-333333333333"
	endpoints := registry.NewMemory()
	for _, endpointID := range []string{endpointA, endpointB} {
		if err := endpoints.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
			t.Fatal(err)
		}
	}
	repoDir := filepath.Join(t.TempDir(), "config")
	writeTestFleetDesired(t, repoDir, "engineering", "configurations:\n  - name: base\n")

	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	store := &restartChangeControlStore{}
	ids := []string{"request-1", "rollout-1", "lease-a", "lease-b"}
	index := 0
	changes, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			id := ids[index]
			index++
			return id
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := changes.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet:          "engineering",
		ReleaseRef:     "release-1",
		ArtifactDigest: "sha256:artifact",
		Targets: []changecontrol.TargetEvidence{
			{EndpointID: endpointA, Compatible: true, PreflightReady: true},
			{EndpointID: endpointB, Compatible: true, PreflightReady: true},
		},
		Resources: []changecontrol.ResourcePlan{{Address: "base/firewall", DesiredHash: "sha256:firewall", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "operator-seed")
	if err != nil || len(requests) != 1 {
		t.Fatalf("seed persisted Change request: requests=%+v err=%v", requests, err)
	}
	serverBeforeRestart := New(Config{Admin: admin, Registry: endpoints, ChangeControl: changes, ConfigRepoPath: repoDir, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	adminRequest := func(srv *Server, method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := adminRequest(serverBeforeRestart, http.MethodPost, "/v1/admin/change-requests/request-1/authorize", []byte(`{"attempt_limit":1,"max_concurrency":1,"justification":"CHG-42"}`)); rec.Code != http.StatusOK {
		t.Fatalf("authorize before restart: status=%d body=%s", rec.Code, rec.Body.String())
	}

	syncEndpoint := func(srv *Server, endpointID string) syncResponse {
		t.Helper()
		body := []byte(`{"changePreflights":[{"change_request_id":"request-1","ready":true}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
		if err != nil {
			t.Fatal(err)
		}
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("sync %s: status=%d body=%s", endpointID, rec.Code, rec.Body.String())
		}
		var response syncResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	first := syncEndpoint(serverBeforeRestart, endpointA)
	if len(first.ExecutionLeases) != 1 || first.ExecutionLeases[0].ID != "lease-a" || first.ExecutionLeases[0].Attempt != 1 {
		t.Fatalf("first lease = %+v", first.ExecutionLeases)
	}

	restored, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	serverAfterRestart := New(Config{Registry: endpoints, ChangeControl: restored, ConfigRepoPath: repoDir})
	if queued := syncEndpoint(serverAfterRestart, endpointB); len(queued.ExecutionLeases) != 0 {
		t.Fatalf("lease issued over restored concurrency limit: %+v", queued.ExecutionLeases)
	}

	now = now.Add(6 * time.Minute)
	afterExpiry := syncEndpoint(serverAfterRestart, endpointB)
	if len(afterExpiry.ExecutionLeases) != 1 || afterExpiry.ExecutionLeases[0].Attempt != 1 {
		t.Fatalf("lease after expiry = %+v", afterExpiry.ExecutionLeases)
	}
	now = now.Add(6 * time.Minute)
	if exhausted := syncEndpoint(serverAfterRestart, endpointA); len(exhausted.ExecutionLeases) != 0 {
		t.Fatalf("consumed attempt was reset by restart: %+v", exhausted.ExecutionLeases)
	}
}

// OS-AEC-078: an Admin API mutation is not acknowledged or retained in memory
// when its durable commit fails, and storage diagnostics are not disclosed.
func TestAdminChangeControlPersistenceFailureLeavesPriorState(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	store := &restartChangeControlStore{}
	ids := []string{"request-1", "rollout-1"}
	index := 0
	changes, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			id := ids[index]
			index++
			return id
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(withDerivedSudoPlan(t, Config{Admin: admin, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM}))
	request := func(server *Server, method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}

	if rec := request(srv, http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", []byte(`{}`)); rec.Code != http.StatusOK {
		t.Fatalf("create: status=%d body=%s", rec.Code, rec.Body.String())
	}
	store.FailNextSave(errors.New("database unavailable: storage-secret-canary"))
	rec := request(srv, http.MethodPost, "/v1/admin/change-requests/request-1/authorize", []byte(`{"attempt_limit":1,"max_concurrency":1,"justification":"CHG-42"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("authorize persistence failure: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("storage-secret-canary")) {
		t.Fatalf("Admin API leaked storage diagnostic: %s", rec.Body.String())
	}

	restored, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	afterRestart := New(Config{Admin: admin, ChangeControl: restored, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	rec = request(afterRestart, http.MethodGet, "/v1/admin/change-requests/request-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("show prior state: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got changecontrol.ChangeRequest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.AuthorizationState != changecontrol.AuthorizationPending || len(got.Approvals) != 0 || len(got.AuditHistory) != 2 {
		t.Fatalf("failed mutation changed durable state: %+v", got)
	}
}

// OS-AEC-078: authenticated Sync reports a failed lease commit and does not
// deliver or consume the unpersisted attempt.
func TestSyncChangeControlPersistenceFailureDeliversNoLease(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	endpoints := registry.NewMemory()
	if err := endpoints.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(t.TempDir(), "config")
	writeTestFleetDesired(t, repoDir, "engineering", "configurations:\n  - name: base\n")

	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	store := &restartChangeControlStore{}
	ids := []string{"request-1", "rollout-1", "lease-1", "lease-2"}
	index := 0
	changes, err := changecontrol.NewPersistentRegistry(context.Background(), store, changecontrol.RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			id := ids[index]
			index++
			return id
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := changes.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet:          "engineering",
		ReleaseRef:     "release-1",
		ArtifactDigest: "sha256:artifact",
		Targets:        []changecontrol.TargetEvidence{{EndpointID: endpointID, Compatible: true, PreflightReady: true}},
		Resources:      []changecontrol.ResourcePlan{{Address: "base/firewall", DesiredHash: "sha256:firewall", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "operator-1")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: requests=%+v err=%v", requests, err)
	}
	if _, err := changes.AuthorizeRollout(requests[0].ID, changecontrol.RolloutSpec{AttemptLimit: 2, MaxConcurrency: 1}, "operator-1", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Registry: endpoints, ChangeControl: changes, ConfigRepoPath: repoDir})

	syncRequest := func() *httptest.ResponseRecorder {
		t.Helper()
		body := []byte(`{"changePreflights":[{"change_request_id":"request-1","ready":true}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
		if err != nil {
			t.Fatal(err)
		}
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	store.FailNextSave(errors.New("database unavailable: storage-secret-canary"))
	failed := syncRequest()
	if failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed lease commit: status=%d body=%s", failed.Code, failed.Body.String())
	}
	if bytes.Contains(failed.Body.Bytes(), []byte("storage-secret-canary")) {
		t.Fatalf("Sync leaked storage diagnostic: %s", failed.Body.String())
	}

	retried := syncRequest()
	if retried.Code != http.StatusOK {
		t.Fatalf("retry: status=%d body=%s", retried.Code, retried.Body.String())
	}
	var response syncResponse
	if err := json.NewDecoder(retried.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.ExecutionLeases) != 1 || response.ExecutionLeases[0].ID != "lease-2" || response.ExecutionLeases[0].Attempt != 1 {
		t.Fatalf("retried lease = %+v", response.ExecutionLeases)
	}
}

type restartChangeControlStore struct {
	mu       sync.Mutex
	payload  []byte
	revision int64
	saveErr  error
}

func (s *restartChangeControlStore) LoadChangeControlState(context.Context) ([]byte, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.payload...), s.revision, nil
}

func (s *restartChangeControlStore) SaveChangeControlState(_ context.Context, expectedRevision int64, payload []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		err := s.saveErr
		s.saveErr = nil
		return 0, err
	}
	if expectedRevision != s.revision {
		return 0, fmt.Errorf("revision conflict: got %d want %d", expectedRevision, s.revision)
	}
	s.revision++
	s.payload = append([]byte(nil), payload...)
	return s.revision, nil
}

func (s *restartChangeControlStore) FailNextSave(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveErr = err
}
