package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type mockDiagnosticsStore struct {
	requests map[string]diagnostics.Request
	byEp     map[string]string
}

func (m *mockDiagnosticsStore) CreateDiagnosticRequest(_ context.Context, endpointID, requestedBy string, spec diagnostics.Spec) (diagnostics.Request, error) {
	if _, ok := m.byEp[endpointID]; ok {
		return diagnostics.Request{}, diagnostics.ErrActiveRequest
	}
	id := uuid.NewString()
	req := diagnostics.Request{
		ID:          id,
		EndpointID:  endpointID,
		RequestedBy: requestedBy,
		Status:      diagnostics.StatusPending,
		Spec:        spec,
		S3Key:       diagnostics.S3Key(endpointID, id),
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(diagnostics.BundleTTL),
	}
	if m.requests == nil {
		m.requests = make(map[string]diagnostics.Request)
	}
	m.requests[id] = req
	m.byEp[endpointID] = id
	return req, nil
}

func (m *mockDiagnosticsStore) GetDiagnosticRequest(_ context.Context, requestID string) (diagnostics.Request, bool, error) {
	req, ok := m.requests[requestID]
	return req, ok, nil
}

func (m *mockDiagnosticsStore) PendingDiagnosticForEndpoint(_ context.Context, endpointID string) (diagnostics.Request, bool, error) {
	id, ok := m.byEp[endpointID]
	if !ok {
		return diagnostics.Request{}, false, nil
	}
	req := m.requests[id]
	if req.Status == diagnostics.StatusReady || req.Status == diagnostics.StatusFailed {
		return diagnostics.Request{}, false, nil
	}
	return req, true, nil
}

func (m *mockDiagnosticsStore) MarkDiagnosticDispatched(_ context.Context, requestID string) error {
	req := m.requests[requestID]
	req.Status = diagnostics.StatusDispatched
	m.requests[requestID] = req
	return nil
}

func (m *mockDiagnosticsStore) MarkDiagnosticRunning(_ context.Context, requestID string) error {
	req := m.requests[requestID]
	req.Status = diagnostics.StatusRunning
	m.requests[requestID] = req
	return nil
}

func (m *mockDiagnosticsStore) CompleteDiagnosticRequest(_ context.Context, result diagnostics.ResultPayload) error {
	req := m.requests[result.RequestID]
	req.Status = result.Status
	req.SHA256 = result.SHA256
	req.SizeBytes = result.SizeBytes
	req.ErrorMessage = result.Message
	m.requests[result.RequestID] = req
	delete(m.byEp, req.EndpointID)
	return nil
}

func (m *mockDiagnosticsStore) ExpireDiagnosticRequests(context.Context) error { return nil }
func (m *mockDiagnosticsStore) DeleteExpiredDiagnosticRequests(context.Context) ([]diagnostics.Request, error) {
	return nil, nil
}

func TestSync_includesDiagnosticCollection(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})
	dstore := &mockDiagnosticsStore{byEp: make(map[string]string)}
	_, _ = dstore.CreateDiagnosticRequest(context.Background(), endpointID, "op", diagnostics.Spec{
		Collectors: diagnostics.DefaultCollectors(),
		Since:      time.Now().UTC().Add(-time.Hour),
		Until:      time.Now().UTC(),
	})

	srv := New(Config{
		ConfigRepoPath: repoDir,
		ReleaseRef:     "dev",
		Registry:       reg,
		Diagnostics:    dstore,
	})

	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	body, _ := json.Marshal(map[string]string{"lastDigest": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.DiagnosticCollection == nil {
		t.Fatal("expected diagnosticCollection")
	}
}

func TestDiagnosticsStore_requiresS3(t *testing.T) {
	srv := New(Config{Diagnostics: &mockDiagnosticsStore{byEp: make(map[string]string)}})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/endpoints/ep/diagnostics/collect", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	_ = apppackages.BlobStore{}
}
