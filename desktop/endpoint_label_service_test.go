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

type endpointLabelMutation struct {
	method string
	path   string
	value  string
}

type endpointLabelServerState struct {
	apiRequests atomic.Int32
	mu          sync.Mutex
	labels      map[string]string
	mutations   []endpointLabelMutation
}

func TestEndpointLabelValidationAndExactMutationContract(t *testing.T) {
	app, serverState := newEndpointLabelTestApp(t)

	invalidSetRequests := []EndpointLabelSetRequest{
		{EndpointID: "", Key: "site", Value: "berlin"},
		{EndpointID: " endpoint-alpha", Key: "site", Value: "berlin"},
		{EndpointID: "endpoint-alpha", Key: ".hidden", Value: "value"},
		{EndpointID: "endpoint-alpha", Key: "bad key", Value: "value"},
		{EndpointID: "endpoint-alpha", Key: "bad=key", Value: "value"},
		{EndpointID: "endpoint-alpha", Key: "bad\tkey", Value: "value"},
		{EndpointID: "endpoint-alpha", Key: strings.Repeat("k", 65), Value: "value"},
		{EndpointID: "endpoint-alpha", Key: "valid", Value: strings.Repeat("v", 513)},
	}
	for _, request := range invalidSetRequests {
		if result, err := app.SetEndpointLabel(request); err == nil {
			t.Fatalf("SetEndpointLabel(%#v) = %#v, want validation error", request, result)
		}
	}
	if result, err := app.RemoveEndpointLabel(EndpointLabelRemoveRequest{
		EndpointID: "endpoint-alpha",
		Key:        ".hidden",
	}); err == nil {
		t.Fatalf("RemoveEndpointLabel(invalid key) = %#v, want validation error", result)
	}
	if got := serverState.apiRequests.Load(); got != 0 {
		t.Fatalf("invalid Label input made %d Admin API request(s), want 0", got)
	}

	boundaryResult, err := app.SetEndpointLabel(EndpointLabelSetRequest{
		EndpointID: "endpoint-alpha",
		Key:        strings.Repeat("k", 64),
		Value:      strings.Repeat("v", 512),
	})
	if err != nil {
		t.Fatalf("set boundary-length Label: %v", err)
	}
	if boundaryResult.Effect != "added" {
		t.Fatalf("boundary Label effect = %q, want added", boundaryResult.Effect)
	}

	added, err := app.SetEndpointLabel(EndpointLabelSetRequest{
		EndpointID: "endpoint-alpha",
		Key:        "site",
		Value:      "berlin",
	})
	if err != nil {
		t.Fatalf("add Label: %v", err)
	}
	if added.Effect != "added" || added.EndpointID != "endpoint-alpha" || added.Key != "site" || added.Value != "berlin" {
		t.Fatalf("added Label result = %#v", added)
	}

	replaced, err := app.SetEndpointLabel(EndpointLabelSetRequest{
		EndpointID: "endpoint-alpha",
		Key:        "environment",
		Value:      "staging",
	})
	if err != nil {
		t.Fatalf("replace Label: %v", err)
	}
	if replaced.Effect != "replaced" || replaced.EndpointID != "endpoint-alpha" || replaced.Key != "environment" || replaced.Value != "staging" {
		t.Fatalf("replaced Label result = %#v", replaced)
	}

	removed, err := app.RemoveEndpointLabel(EndpointLabelRemoveRequest{
		EndpointID: "endpoint-alpha",
		Key:        "region",
	})
	if err != nil {
		t.Fatalf("remove Label: %v", err)
	}
	if removed.Effect != "removed" || removed.EndpointID != "endpoint-alpha" || removed.Key != "region" || removed.Value != "" {
		t.Fatalf("removed Label result = %#v", removed)
	}
	for _, label := range removed.Labels {
		if label.Key == "region" {
			t.Fatalf("removed result retained region Label: %#v", removed.Labels)
		}
	}
	for _, want := range []LabelView{
		{Key: "environment", Value: "staging"},
		{Key: "site", Value: "berlin"},
	} {
		found := false
		for _, label := range removed.Labels {
			if reflect.DeepEqual(label, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("removed result lost unrelated Label %#v: %#v", want, removed.Labels)
		}
	}

	serverState.mu.Lock()
	mutations := append([]endpointLabelMutation(nil), serverState.mutations...)
	serverState.mu.Unlock()
	wantMutations := []endpointLabelMutation{
		{method: http.MethodPut, path: "/v1/admin/endpoints/endpoint-alpha/labels/" + strings.Repeat("k", 64), value: strings.Repeat("v", 512)},
		{method: http.MethodPut, path: "/v1/admin/endpoints/endpoint-alpha/labels/site", value: "berlin"},
		{method: http.MethodPut, path: "/v1/admin/endpoints/endpoint-alpha/labels/environment", value: "staging"},
		{method: http.MethodDelete, path: "/v1/admin/endpoints/endpoint-alpha/labels/region"},
	}
	if !reflect.DeepEqual(mutations, wantMutations) {
		t.Fatalf("Label mutations = %#v, want %#v", mutations, wantMutations)
	}
}

func newEndpointLabelTestApp(t *testing.T) (*App, *endpointLabelServerState) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &endpointLabelServerState{
		labels: map[string]string{
			"environment": "production",
			"region":      "west",
		},
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-labels","roles":["global_admin"]}`))
			return
		}
		if !strings.HasPrefix(request.URL.Path, "/v1/admin/endpoints/") {
			http.NotFound(response, request)
			return
		}
		serverState.apiRequests.Add(1)

		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/endpoints/endpoint-alpha" {
			serverState.mu.Lock()
			labels := cloneStringMap(serverState.labels)
			serverState.mu.Unlock()
			writeEndpointLabelTestJSON(response, map[string]any{
				"id":     "endpoint-alpha",
				"fleet":  "production",
				"labels": labels,
			})
			return
		}

		const labelPrefix = "/v1/admin/endpoints/endpoint-alpha/labels/"
		if !strings.HasPrefix(request.URL.Path, labelPrefix) {
			http.NotFound(response, request)
			return
		}
		key := strings.TrimPrefix(request.URL.Path, labelPrefix)
		switch request.Method {
		case http.MethodPut:
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read Label request: %v", err)
				http.Error(response, "read failed", http.StatusInternalServerError)
				return
			}
			var payload struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode Label request: %v", err)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			serverState.mu.Lock()
			serverState.labels[key] = payload.Value
			serverState.mutations = append(serverState.mutations, endpointLabelMutation{method: request.Method, path: request.URL.Path, value: payload.Value})
			labels := cloneStringMap(serverState.labels)
			serverState.mu.Unlock()
			writeEndpointLabelTestJSON(response, map[string]any{
				"key": key, "value": payload.Value, "labels": labels,
			})
		case http.MethodDelete:
			serverState.mu.Lock()
			delete(serverState.labels, key)
			serverState.mutations = append(serverState.mutations, endpointLabelMutation{method: request.Method, path: request.URL.Path})
			serverState.mu.Unlock()
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
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
		"operator-labels",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Label Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, serverState
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func writeEndpointLabelTestJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(value)
}
