package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fleetUpgradeMutation struct {
	path    string
	version string
}

type fleetUpgradeServerState struct {
	apiRequests atomic.Int32
	mu          sync.Mutex
	mutations   []fleetUpgradeMutation
}

func TestFleetUpgradeExactRequestAndServerCountContract(t *testing.T) {
	app, serverState := newFleetUpgradeTestApp(t)

	for _, request := range []FleetUpgradeRequest{
		{Fleet: "", Version: "v2.2.0"},
		{Fleet: " production", Version: "v2.2.0"},
		{Fleet: "production", Version: ""},
		{Fleet: "production", Version: "latest"},
	} {
		if result, err := app.RequestFleetAgentUpgrade(request); err == nil {
			t.Fatalf("RequestFleetAgentUpgrade(%#v) = %#v, want validation error", request, result)
		}
	}
	if got := serverState.apiRequests.Load(); got != 0 {
		t.Fatalf("invalid Fleet upgrade input made %d Admin API request(s), want 0", got)
	}

	result, err := app.RequestFleetAgentUpgrade(FleetUpgradeRequest{
		Fleet:   "production",
		Version: "v2.2.0",
	})
	if err != nil {
		t.Fatalf("request Fleet upgrade: %v", err)
	}
	if result.Status != "requested" || result.Fleet != "production" || result.Version != "v2.2.0" {
		t.Fatalf("Fleet upgrade result = %#v", result)
	}
	if result.AcceptedEndpoints != 3 {
		t.Fatalf("accepted Endpoints = %d, want server-returned 3", result.AcceptedEndpoints)
	}

	serverState.mu.Lock()
	mutations := append([]fleetUpgradeMutation(nil), serverState.mutations...)
	serverState.mu.Unlock()
	want := []fleetUpgradeMutation{{
		path:    "/v1/admin/fleets/production/agent-upgrade",
		version: "v2.2.0",
	}}
	if !reflect.DeepEqual(mutations, want) {
		t.Fatalf("Fleet upgrade mutations = %#v, want %#v", mutations, want)
	}
}

func newFleetUpgradeTestApp(t *testing.T) (*App, *fleetUpgradeServerState) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &fleetUpgradeServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-fleet-upgrade","roles":["global_admin"]}`))
			return
		}

		serverState.apiRequests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/admin/fleets/production/agent-upgrade" {
			http.NotFound(response, request)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Fleet upgrade request: %v", err)
			http.Error(response, "read failed", http.StatusInternalServerError)
			return
		}
		var payload struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode Fleet upgrade request: %v", err)
			http.Error(response, "bad request", http.StatusBadRequest)
			return
		}
		serverState.mu.Lock()
		serverState.mutations = append(serverState.mutations, fleetUpgradeMutation{
			path:    request.URL.Path,
			version: payload.Version,
		})
		serverState.mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"version":   payload.Version,
			"endpoints": 3,
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
		"operator-fleet-upgrade",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Fleet upgrade Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, serverState
}
