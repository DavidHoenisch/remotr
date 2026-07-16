package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type diagnosticCollectionMutation struct {
	collectors []string
	endpointID string
	since      string
	until      string
}

type diagnosticCollectionServerState struct {
	apiRequests atomic.Int32
	conflict    atomic.Bool
	mu          sync.Mutex
	mutations   []diagnosticCollectionMutation
}

func TestDiagnosticCollectionExactRequestAndValidationContract(t *testing.T) {
	app, serverState := newDiagnosticCollectionTestApp(t)

	wantCapabilities := DiagnosticCapabilities{
		Collectors: []string{
			"system_info",
			"network_state",
			"journal_remotr",
			"journal_kernel",
			"journal_audit",
			"dmesg",
			"remotr_agent_state",
		},
		MaxTimeSpanSeconds: 7 * 24 * 60 * 60,
	}
	if got := app.GetDiagnosticCapabilities(); !reflect.DeepEqual(got, wantCapabilities) {
		t.Fatalf("diagnostic capabilities = %#v, want %#v", got, wantCapabilities)
	}

	validSince := "2026-02-25T05:05:07Z"
	validUntil := "2026-03-04T05:05:07Z"
	invalidRequests := []DiagnosticCollectionRequest{
		{EndpointID: "", Collectors: []string{"system_info"}, Since: validSince, Until: validUntil},
		{EndpointID: " endpoint-alpha", Collectors: []string{"system_info"}, Since: validSince, Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: nil, Since: validSince, Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{""}, Since: validSince, Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"shell_history"}, Since: validSince, Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"system_info"}, Since: "", Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"system_info"}, Since: "yesterday", Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"system_info"}, Since: validUntil, Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"system_info"}, Since: "2026-03-05T05:05:07Z", Until: validUntil},
		{EndpointID: "endpoint-alpha", Collectors: []string{"system_info"}, Since: "2026-02-25T05:05:06Z", Until: validUntil},
	}
	for _, request := range invalidRequests {
		if result, err := app.RequestDiagnosticCollection(request); err == nil {
			t.Errorf("RequestDiagnosticCollection(%#v) = %#v, want validation error", request, result)
		}
	}
	if got := serverState.apiRequests.Load(); got != 0 {
		t.Fatalf("invalid diagnostic input made %d Admin API request(s), want 0", got)
	}

	result, err := app.RequestDiagnosticCollection(DiagnosticCollectionRequest{
		EndpointID: "endpoint-alpha",
		Collectors: []string{"network_state", "journal_kernel"},
		Since:      validSince,
		Until:      validUntil,
	})
	if err != nil {
		t.Fatalf("request diagnostic collection: %v", err)
	}
	wantResult := DiagnosticCollectionResult{
		RequestID:  "diagnostic-42",
		EndpointID: "endpoint-alpha",
		Status:     "pending",
		Collectors: []string{"network_state", "journal_kernel"},
		Since:      validSince,
		Until:      validUntil,
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("diagnostic result = %#v, want %#v", result, wantResult)
	}

	serverState.mu.Lock()
	mutations := append([]diagnosticCollectionMutation(nil), serverState.mutations...)
	serverState.mu.Unlock()
	wantMutations := []diagnosticCollectionMutation{{
		collectors: []string{"network_state", "journal_kernel"},
		endpointID: "endpoint-alpha",
		since:      validSince,
		until:      validUntil,
	}}
	if !reflect.DeepEqual(mutations, wantMutations) {
		t.Fatalf("diagnostic mutations = %#v, want %#v", mutations, wantMutations)
	}
	if got := serverState.apiRequests.Load(); got != 1 {
		t.Fatalf("valid diagnostic input made %d Admin API request(s), want 1", got)
	}
}

func TestDiagnosticCollectionActiveConflictIsNotRetried(t *testing.T) {
	app, serverState := newDiagnosticCollectionTestApp(t)
	serverState.conflict.Store(true)

	_, err := app.RequestDiagnosticCollection(DiagnosticCollectionRequest{
		EndpointID: "endpoint-alpha",
		Collectors: []string{"system_info"},
		Since:      "2026-03-03T05:05:07Z",
		Until:      "2026-03-04T05:05:07Z",
	})
	var failure *ActionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("diagnostic conflict error = %T %v, want ActionFailure", err, err)
	}
	if failure.Kind != ActionConflict || failure.Retryable {
		t.Fatalf("diagnostic conflict = %#v, want non-retryable conflict", failure)
	}
	if got := serverState.apiRequests.Load(); got != 1 {
		t.Fatalf("diagnostic conflict made %d Admin API request(s), want exactly 1", got)
	}
}

func FuzzDiagnosticCollectionIntervalValidation(f *testing.F) {
	for _, seed := range [][2]string{
		{"", ""},
		{"yesterday", "2026-03-04T05:05:07Z"},
		{"2026-03-04T05:05:07Z", "2026-03-04T05:05:07Z"},
		{"2026-03-05T05:05:07Z", "2026-03-04T05:05:07Z"},
		{"2026-02-25T05:05:07Z", "2026-03-04T05:05:07Z"},
		{"2026-02-25T05:05:06Z", "2026-03-04T05:05:07Z"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, rawSince, rawUntil string) {
		if len(rawSince) > 128 || len(rawUntil) > 128 {
			// test-exception: EXC-022
			t.Skip()
		}
		since, sinceErr := time.Parse(time.RFC3339, rawSince)
		until, untilErr := time.Parse(time.RFC3339, rawUntil)
		valid := sinceErr == nil && untilErr == nil && until.After(since) && until.Sub(since) <= 7*24*time.Hour

		_, err := NewDiagnosticCollectionService().RequestConnected(t.Context(), nil, DiagnosticCollectionRequest{
			EndpointID: "endpoint-alpha",
			Collectors: []string{"system_info"},
			Since:      rawSince,
			Until:      rawUntil,
		})
		if valid {
			if !errors.Is(err, ErrSessionNotConnected) {
				t.Fatalf("valid absolute interval (%q, %q) error = %T %v, want disconnected boundary", rawSince, rawUntil, err, err)
			}
			return
		}
		var failure *ActionFailure
		if !errors.As(err, &failure) || failure.Kind != ActionValidation {
			t.Fatalf("invalid absolute interval (%q, %q) error = %T %v, want validation failure", rawSince, rawUntil, err, err)
		}
	})
}

func newDiagnosticCollectionTestApp(t *testing.T) (*App, *diagnosticCollectionServerState) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &diagnosticCollectionServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-diagnostics","roles":["global_admin"]}`))
			return
		}

		serverState.apiRequests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/admin/endpoints/endpoint-alpha/diagnostics/collect" {
			http.NotFound(response, request)
			return
		}
		if serverState.conflict.Load() {
			http.Error(response, "endpoint already has an active diagnostic request", http.StatusConflict)
			return
		}
		var payload struct {
			Collectors []string `json:"collectors"`
			Since      string   `json:"since"`
			Until      string   `json:"until"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode diagnostic request: %v", err)
			http.Error(response, "bad request", http.StatusBadRequest)
			return
		}
		serverState.mu.Lock()
		serverState.mutations = append(serverState.mutations, diagnosticCollectionMutation{
			collectors: append([]string(nil), payload.Collectors...),
			endpointID: "endpoint-alpha",
			since:      payload.Since,
			until:      payload.Until,
		})
		serverState.mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id":          "diagnostic-42",
			"endpoint_id": "endpoint-alpha",
			"status":      "pending",
			"spec": map[string]any{
				"collectors": payload.Collectors,
				"since":      payload.Since,
				"until":      payload.Until,
			},
			"created_at": "2026-03-04T05:05:08Z",
			"expires_at": "2026-03-05T05:05:08Z",
		})
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    connectionCertPool(t, fixture.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time: func() time.Time {
			return connectionTestTime
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(
		t,
		"operator-diagnostics",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect diagnostic Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, serverState
}
