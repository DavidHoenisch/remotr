package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type mockDiagnosticsStore struct {
	requests       map[string]diagnostics.Request
	byEp           map[string]string
	getErr         error
	markRunningErr error
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
	if m.getErr != nil {
		return diagnostics.Request{}, false, m.getErr
	}
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
	if m.markRunningErr != nil {
		return m.markRunningErr
	}
	req := m.requests[requestID]
	req.Status = diagnostics.StatusRunning
	m.requests[requestID] = req
	return nil
}

type diagnosticUploadURLStub struct {
	url   string
	err   error
	key   string
	ttl   time.Duration
	calls int
}

func (s *diagnosticUploadURLStub) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	s.calls++
	s.key = key
	s.ttl = ttl
	return s.url, s.err
}

func (m *mockDiagnosticsStore) CompleteDiagnosticRequest(_ context.Context, result diagnostics.ResultPayload) error {
	req := m.requests[result.RequestID]
	req.Status = result.Status
	req.SHA256 = result.SHA256
	req.SizeBytes = result.SizeBytes
	req.Failure = result.Failure
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
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
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

func TestDiagnosticUploadURLRejectsInvalidPublicRequests(t *testing.T) {
	const (
		endpointID = "11111111-1111-1111-1111-111111111111"
		requestID  = "22222222-2222-2222-2222-222222222222"
	)
	blobs := serverTestDiagnosticBlobStore(t)

	tests := []struct {
		name            string
		store           *mockDiagnosticsStore
		blobs           *apppackages.BlobStore
		provider        *diagnosticUploadURLStub
		body            string
		authenticated   bool
		wantStatus      int
		wantPresignCall int
	}{
		{
			name:       "missing dependencies",
			store:      &mockDiagnosticsStore{},
			body:       `{"requestId":"` + requestID + `"}`,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "missing endpoint identity",
			store:      diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusDispatched),
			blobs:      blobs,
			provider:   &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:       `{"requestId":"` + requestID + `"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "missing request id",
			store:         diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusDispatched),
			blobs:         blobs,
			provider:      &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:          `{}`,
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name: "lookup failure",
			store: &mockDiagnosticsStore{
				getErr: errors.New("persistence unavailable"),
			},
			blobs:         blobs,
			provider:      &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:          `{"requestId":"` + requestID + `"}`,
			authenticated: true,
			wantStatus:    http.StatusInternalServerError,
		},
		{
			name:          "unknown request",
			store:         &mockDiagnosticsStore{requests: make(map[string]diagnostics.Request)},
			blobs:         blobs,
			provider:      &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:          `{"requestId":"` + requestID + `"}`,
			authenticated: true,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "request owned by another endpoint",
			store:         diagnosticUploadTestStore("33333333-3333-3333-3333-333333333333", requestID, diagnostics.StatusDispatched),
			blobs:         blobs,
			provider:      &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:          `{"requestId":"` + requestID + `"}`,
			authenticated: true,
			wantStatus:    http.StatusForbidden,
		},
		{
			name:            "request not accepting uploads",
			store:           diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusPending),
			blobs:           blobs,
			provider:        &diagnosticUploadURLStub{url: "https://upload.example.invalid"},
			body:            `{"requestId":"` + requestID + `"}`,
			authenticated:   true,
			wantStatus:      http.StatusConflict,
			wantPresignCall: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(Config{
				Diagnostics:          tt.store,
				AppPackageBlobs:      tt.blobs,
				DiagnosticUploadURLs: tt.provider,
			})
			rec := serveDiagnosticUploadURL(t, srv, endpointID, tt.body, tt.authenticated)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.provider != nil && tt.provider.calls != tt.wantPresignCall {
				t.Fatalf("presign calls = %d, want %d", tt.provider.calls, tt.wantPresignCall)
			}
		})
	}
}

func TestDiagnosticUploadURLUsesDefaultTTLAndMarksRequestRunning(t *testing.T) {
	const (
		endpointID = "11111111-1111-1111-1111-111111111111"
		requestID  = "22222222-2222-2222-2222-222222222222"
	)
	fixedNow := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	store := diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusDispatched)
	provider := &diagnosticUploadURLStub{url: "https://upload.example.invalid/grant"}
	srv := New(Config{
		Diagnostics:          store,
		AppPackageBlobs:      serverTestDiagnosticBlobStore(t),
		DiagnosticUploadURLs: provider,
		Now:                  func() time.Time { return fixedNow },
	})

	rec := serveDiagnosticUploadURL(t, srv, endpointID, `{"requestId":"`+requestID+`"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response diagnosticUploadURLResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.URL != provider.url || response.Key != diagnostics.S3Key(endpointID, requestID) {
		t.Fatalf("upload response = %#v", response)
	}
	if provider.calls != 1 || provider.key != response.Key || provider.ttl != 30*time.Minute {
		t.Fatalf("presign = calls:%d key:%q ttl:%s", provider.calls, provider.key, provider.ttl)
	}
	if want := fixedNow.Add(30 * time.Minute); !response.ExpiresAt.Equal(want) {
		t.Fatalf("expires at = %s, want %s", response.ExpiresAt, want)
	}
	if got := store.requests[requestID].Status; got != diagnostics.StatusRunning {
		t.Fatalf("request status = %q, want %q", got, diagnostics.StatusRunning)
	}
}

func TestDiagnosticUploadURLClassifiesExternalFailures(t *testing.T) {
	const (
		endpointID = "11111111-1111-1111-1111-111111111111"
		requestID  = "22222222-2222-2222-2222-222222222222"
	)
	blobs := serverTestDiagnosticBlobStore(t)
	tests := []struct {
		name      string
		store     *mockDiagnosticsStore
		provider  *diagnosticUploadURLStub
		wantCalls int
	}{
		{
			name:      "presign failure",
			store:     diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusDispatched),
			provider:  &diagnosticUploadURLStub{err: errors.New("signer unavailable")},
			wantCalls: 1,
		},
		{
			name: "running-state persistence failure",
			store: func() *mockDiagnosticsStore {
				store := diagnosticUploadTestStore(endpointID, requestID, diagnostics.StatusDispatched)
				store.markRunningErr = errors.New("persistence unavailable")
				return store
			}(),
			provider:  &diagnosticUploadURLStub{url: "https://upload.example.invalid/grant"},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(Config{
				Diagnostics:          tt.store,
				AppPackageBlobs:      blobs,
				DiagnosticUploadURLs: tt.provider,
			})
			rec := serveDiagnosticUploadURL(t, srv, endpointID, `{"requestId":"`+requestID+`"}`, true)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			if tt.provider.calls != tt.wantCalls {
				t.Fatalf("presign calls = %d, want %d", tt.provider.calls, tt.wantCalls)
			}
			if got := tt.store.requests[requestID].Status; got != diagnostics.StatusDispatched {
				t.Fatalf("request status = %q, want unchanged %q", got, diagnostics.StatusDispatched)
			}
		})
	}
}

func diagnosticUploadTestStore(endpointID, requestID, status string) *mockDiagnosticsStore {
	return &mockDiagnosticsStore{
		requests: map[string]diagnostics.Request{
			requestID: {
				ID: requestID, EndpointID: endpointID, Status: status,
				S3Key: diagnostics.S3Key(endpointID, requestID),
			},
		},
		byEp: map[string]string{endpointID: requestID},
	}
}

func serveDiagnosticUploadURL(t *testing.T, srv *Server, endpointID, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/upload-url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authenticated {
		uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
		if err != nil {
			t.Fatal(err)
		}
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func serverTestDiagnosticBlobStore(t *testing.T) *apppackages.BlobStore {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	blobs, err := apppackages.NewBlobStore(t.Context(), apppackages.S3Config{
		Bucket: "test", Region: "us-east-1", Endpoint: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return blobs
}

func TestPersistDiagnosticResultAdmitsOnlyClassifiedBundleBeforeReady(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	validBundle := serverTestDiagnosticBundle(t)
	tests := []struct {
		name       string
		bundle     []byte
		wantReady  bool
		wantDelete bool
	}{
		{name: "classified bundle", bundle: validBundle, wantReady: true},
		{name: "raw invalid bundle", bundle: []byte("raw-diagnostic-secret-canary"), wantDelete: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted := false
			objectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.Header().Set("Content-Length", strconv.Itoa(len(tt.bundle)))
					_, _ = w.Write(tt.bundle)
				case http.MethodDelete:
					deleted = true
					w.WriteHeader(http.StatusNoContent)
				default:
					http.Error(w, "unsupported", http.StatusMethodNotAllowed)
				}
			}))
			defer objectServer.Close()

			t.Setenv("AWS_ACCESS_KEY_ID", "test")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
			blobs, err := apppackages.NewBlobStore(t.Context(), apppackages.S3Config{
				Bucket: "test", Region: "us-east-1", Endpoint: objectServer.URL,
			})
			if err != nil {
				t.Fatal(err)
			}
			requestID := uuid.NewString()
			digest := sha256.Sum256(tt.bundle)
			dstore := &mockDiagnosticsStore{
				requests: map[string]diagnostics.Request{
					requestID: {
						ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning,
						S3Key: diagnostics.S3Key(endpointID, requestID),
					},
				},
				byEp: map[string]string{endpointID: requestID},
			}
			srv := New(Config{Diagnostics: dstore, AppPackageBlobs: blobs})
			srv.persistDiagnosticResult(t.Context(), endpointID, &diagnosticResultPayload{
				RequestID: requestID,
				Status:    diagnostics.StatusReady,
				SHA256:    hex.EncodeToString(digest[:]),
				SizeBytes: int64(len(tt.bundle)),
			})

			stored := dstore.requests[requestID]
			if tt.wantReady {
				if stored.Status != diagnostics.StatusReady || stored.SHA256 == "" || stored.SizeBytes != int64(len(tt.bundle)) || stored.Failure != nil {
					t.Fatalf("classified bundle completion = %#v", stored)
				}
			} else {
				if stored.Status != diagnostics.StatusFailed || stored.SHA256 != "" || stored.SizeBytes != 0 || stored.Failure == nil || stored.Failure.ReasonCode != "diagnostic_bundle_invalid" {
					t.Fatalf("rejected bundle completion = %#v", stored)
				}
			}
			if deleted != tt.wantDelete {
				t.Fatalf("object deleted = %t, want %t", deleted, tt.wantDelete)
			}
		})
	}
}

func TestPersistDiagnosticResultClassifiesInvalidStatusAndRejectsWrongEndpoint(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	requestID := uuid.NewString()
	dstore := &mockDiagnosticsStore{
		requests: map[string]diagnostics.Request{
			requestID: {ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning},
		},
		byEp: map[string]string{endpointID: requestID},
	}
	srv := New(Config{Diagnostics: dstore})
	srv.persistDiagnosticResult(t.Context(), endpointID, &diagnosticResultPayload{RequestID: requestID, Status: "unexpected"})
	stored := dstore.requests[requestID]
	if stored.Status != diagnostics.StatusFailed || stored.Failure == nil || stored.Failure.ReasonCode != "diagnostic_status_invalid" {
		t.Fatalf("invalid status completion = %#v", stored)
	}

	reportedFailure := executor.NewSafeError("agent_reported_failure", "diagnostic_collection", nil)
	dstore.requests[requestID] = diagnostics.Request{ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning}
	dstore.byEp[endpointID] = requestID
	srv.persistDiagnosticResult(t.Context(), endpointID, &diagnosticResultPayload{
		RequestID: requestID, Status: "aaa", Failure: &reportedFailure,
	})
	stored = dstore.requests[requestID]
	if stored.Status != diagnostics.StatusFailed || stored.Failure == nil || stored.Failure.ReasonCode != reportedFailure.ReasonCode {
		t.Fatalf("invalid status did not preserve classified agent failure: %#v", stored)
	}

	dstore.requests[requestID] = diagnostics.Request{ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning}
	dstore.byEp[endpointID] = requestID
	srv.persistDiagnosticResult(t.Context(), endpointID, &diagnosticResultPayload{
		RequestID: requestID, Status: diagnostics.StatusFailed, Failure: &reportedFailure,
	})
	stored = dstore.requests[requestID]
	if stored.Status != diagnostics.StatusFailed || stored.Failure == nil || stored.Failure.ReasonCode != reportedFailure.ReasonCode {
		t.Fatalf("classified failed result was reinterpreted as a ready bundle: %#v", stored)
	}

	dstore.requests[requestID] = diagnostics.Request{
		ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning,
		S3Key: diagnostics.S3Key(endpointID, requestID),
	}
	dstore.byEp[endpointID] = requestID
	srv.persistDiagnosticResult(t.Context(), endpointID, &diagnosticResultPayload{
		RequestID: requestID, Status: diagnostics.StatusReady, SHA256: strings.Repeat("a", 64), SizeBytes: 1,
	})
	stored = dstore.requests[requestID]
	if stored.Status != diagnostics.StatusFailed || stored.Failure == nil || stored.Failure.ReasonCode != "diagnostic_bundle_invalid" {
		t.Fatalf("ready result without blob store was admitted: %#v", stored)
	}

	dstore.requests[requestID] = diagnostics.Request{ID: requestID, EndpointID: endpointID, Status: diagnostics.StatusRunning}
	srv.persistDiagnosticResult(t.Context(), "22222222-2222-2222-2222-222222222222", &diagnosticResultPayload{RequestID: requestID, Status: diagnostics.StatusFailed})
	if got := dstore.requests[requestID].Status; got != diagnostics.StatusRunning {
		t.Fatalf("wrong endpoint changed diagnostic status to %q", got)
	}
}

func serverTestDiagnosticBundle(t *testing.T) []byte {
	t.Helper()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	digest := sha256.Sum256([]byte("source bytes"))
	byteCount := 12
	lineCount := 1
	collected := true
	summary, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &byteCount},
		{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &collected},
		{Path: "lines", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &lineCount},
		{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: hex.EncodeToString(digest[:])},
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(diagnostics.BundleManifest{
		RequestID: "request-1", Collectors: []string{diagnostics.CollectorJournalRemotr},
		Since: now.Add(-time.Hour), Until: now, CollectedAt: now,
		Files: []string{"journal/remotr-agent.summary.json"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range map[string][]byte{
		"manifest.json":                     manifestJSON,
		"journal/remotr-agent.summary.json": summaryJSON,
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
