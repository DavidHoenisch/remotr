package main

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type endpointRemovalServerState struct {
	apiRequests atomic.Int32
	mu          sync.Mutex
	paths       []string
}

func TestEndpointRemovalRequiresExactIndependentConfirmation(t *testing.T) {
	app, serverState := newEndpointRemovalTestApp(t)

	for _, request := range []EndpointRemovalRequest{
		{EndpointID: "", Confirmation: ""},
		{EndpointID: " endpoint-alpha", Confirmation: " endpoint-alpha"},
		{EndpointID: "endpoint-alpha", Confirmation: ""},
		{EndpointID: "endpoint-alpha", Confirmation: "Endpoint-Alpha"},
		{EndpointID: "endpoint-alpha", Confirmation: "endpoint-alpha "},
		{EndpointID: "endpoint-alpha", Confirmation: "endpoint-beta"},
	} {
		if result, err := app.RemoveEndpoint(request); err == nil {
			t.Errorf("RemoveEndpoint(%#v) = %#v, want validation error", request, result)
		}
	}
	if got := serverState.apiRequests.Load(); got != 0 {
		t.Fatalf("invalid Endpoint removal made %d Admin API request(s), want 0", got)
	}

	result, err := app.RemoveEndpoint(EndpointRemovalRequest{
		EndpointID:   "endpoint-alpha",
		Confirmation: "endpoint-alpha",
	})
	if err != nil {
		t.Fatalf("remove exact Endpoint: %v", err)
	}
	want := EndpointRemovalResult{
		Status:           "removed",
		EndpointID:       "endpoint-alpha",
		CredentialStatus: "not_enrolled",
		AffectedEvidence: []string{"inventory", "activity"},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("Endpoint removal result = %#v, want %#v", result, want)
	}
	if got := serverState.apiRequests.Load(); got != 1 {
		t.Fatalf("exact Endpoint removal made %d Admin API request(s), want 1", got)
	}
	serverState.mu.Lock()
	paths := append([]string(nil), serverState.paths...)
	serverState.mu.Unlock()
	if !reflect.DeepEqual(paths, []string{"/v1/admin/endpoints/endpoint-alpha"}) {
		t.Fatalf("Endpoint removal paths = %v, want exact target", paths)
	}
}

func FuzzEndpointRemovalConfirmationIsExact(f *testing.F) {
	for _, confirmation := range []string{
		"",
		"endpoint-alpha",
		"Endpoint-Alpha",
		"endpoint-alpha ",
		"endpoint-beta",
	} {
		f.Add(confirmation)
	}
	f.Fuzz(func(t *testing.T, confirmation string) {
		if len(confirmation) > 128 {
			t.Skip()
		}
		_, err := NewEndpointRemovalService().RemoveConnected(t.Context(), nil, EndpointRemovalRequest{
			EndpointID:   "endpoint-alpha",
			Confirmation: confirmation,
		})
		if confirmation == "endpoint-alpha" {
			if !errors.Is(err, ErrSessionNotConnected) {
				t.Fatalf("exact confirmation error = %T %v, want disconnected boundary", err, err)
			}
			return
		}
		var failure *ActionFailure
		if !errors.As(err, &failure) || failure.Kind != ActionValidation {
			t.Fatalf("non-exact confirmation %q error = %T %v, want validation failure", confirmation, err, err)
		}
	})
}

func newEndpointRemovalTestApp(t *testing.T) (*App, *endpointRemovalServerState) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &endpointRemovalServerState{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-endpoint-removal","roles":["global_admin"]}`))
			return
		}
		serverState.apiRequests.Add(1)
		serverState.mu.Lock()
		serverState.paths = append(serverState.paths, request.URL.Path)
		serverState.mu.Unlock()
		if request.Method != http.MethodDelete || request.URL.Path != "/v1/admin/endpoints/endpoint-alpha" {
			http.NotFound(response, request)
			return
		}
		response.WriteHeader(http.StatusNoContent)
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
		"operator-endpoint-removal",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Endpoint removal Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, serverState
}
