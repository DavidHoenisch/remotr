package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type endpointUpgradeMutation struct {
	path    string
	version string
}

type endpointUpgradeServerState struct {
	apiRequests atomic.Int32
	mu          sync.Mutex
	mutations   []endpointUpgradeMutation
}

func TestEndpointUpgradeExactRequestContract(t *testing.T) {
	app, serverState := newEndpointUpgradeTestApp(t)

	for _, request := range []EndpointUpgradeRequest{
		{EndpointID: "", Version: "v2.2.0"},
		{EndpointID: " endpoint-alpha", Version: "v2.2.0"},
		{EndpointID: "endpoint-alpha", Version: ""},
		{EndpointID: "endpoint-alpha", Version: "   "},
	} {
		if result, err := app.RequestEndpointAgentUpgrade(request); err == nil {
			t.Fatalf("RequestEndpointAgentUpgrade(%#v) = %#v, want validation error", request, result)
		}
	}
	if got := serverState.apiRequests.Load(); got != 0 {
		t.Fatalf("invalid Endpoint upgrade input made %d Admin API request(s), want 0", got)
	}

	result, err := app.RequestEndpointAgentUpgrade(EndpointUpgradeRequest{
		EndpointID: "endpoint-alpha",
		Version:    "v2.2.0",
	})
	if err != nil {
		t.Fatalf("request Endpoint upgrade: %v", err)
	}
	if result.Status != "requested" || result.EndpointID != "endpoint-alpha" || result.Version != "v2.2.0" {
		t.Fatalf("Endpoint upgrade result = %#v", result)
	}
	if !reflect.DeepEqual(result.AffectedEvidence, []string{"desired_agent_version", "reported_agent_version", "activity"}) {
		t.Fatalf("affected evidence = %#v", result.AffectedEvidence)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode Endpoint upgrade result: %v", err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), `"status":"completed"`) {
		t.Fatalf("request result claimed completion: %s", encoded)
	}

	serverState.mu.Lock()
	mutations := append([]endpointUpgradeMutation(nil), serverState.mutations...)
	serverState.mu.Unlock()
	want := []endpointUpgradeMutation{{
		path:    "/v1/admin/endpoints/endpoint-alpha/agent-upgrade",
		version: "v2.2.0",
	}}
	if !reflect.DeepEqual(mutations, want) {
		t.Fatalf("Endpoint upgrade mutations = %#v, want %#v", mutations, want)
	}
}

func newEndpointUpgradeTestApp(t *testing.T) (*App, *endpointUpgradeServerState) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &endpointUpgradeServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-upgrade","roles":["global_admin"]}`))
			return
		}

		serverState.apiRequests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/admin/endpoints/endpoint-alpha/agent-upgrade" {
			http.NotFound(response, request)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Endpoint upgrade request: %v", err)
			http.Error(response, "read failed", http.StatusInternalServerError)
			return
		}
		var payload struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode Endpoint upgrade request: %v", err)
			http.Error(response, "bad request", http.StatusBadRequest)
			return
		}
		serverState.mu.Lock()
		serverState.mutations = append(serverState.mutations, endpointUpgradeMutation{
			path:    request.URL.Path,
			version: payload.Version,
		})
		serverState.mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]string{"version": payload.Version})
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
		"operator-upgrade",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect upgrade Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, serverState
}
