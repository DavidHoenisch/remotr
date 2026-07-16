package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const authorizationResponseCanary = "authorization-forbidden-response-canary"

func TestServerAuthoritativeForbiddenActionKeepsConnectedSession(t *testing.T) {
	manager, identityRequests, actionRequests, stateDir := newAuthorizationTestSession(t)
	before := manager.Snapshot()
	if before.Status != SessionConnected || before.Identity == nil {
		t.Fatalf("initial session = %#v, want connected Operator identity", before)
	}
	if !slices.Equal(before.Identity.Roles, []string{"read_only"}) {
		t.Fatalf("presented roles = %v, want [read_only]", before.Identity.Roles)
	}

	// The read-only role hint would normally hide Git sync. Deliberately invoke
	// the typed backend action anyway to prove frontend state is not authority.
	if slices.Contains(before.Identity.Roles, "global_admin") {
		t.Fatal("controlled role unexpectedly makes the Git sync hint available")
	}
	err := manager.ExecuteAuthenticatedAction(t.Context(), func(ctx context.Context, client *admin.Client) error {
		return client.TriggerGitSyncContext(ctx)
	})
	assertForbiddenActionFailure(t, err, stateDir)

	after := manager.Snapshot()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("session changed after forbidden action\n before: %#v\n  after: %#v", before, after)
	}
	if got := identityRequests.Load(); got != 1 {
		t.Fatalf("identity requests = %d, want 1 without alternate-identity retry", got)
	}
	if got := actionRequests.Load(); got != 1 {
		t.Fatalf("forbidden action requests = %d, want exactly 1", got)
	}
}

func TestEveryDesktopMutationPreservesStateWhenServerForbidsIt(t *testing.T) {
	app, actionRequests := newCrossActionAuthorizationTestApp(t)
	before := app.sessions.Snapshot()

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "Git sync", run: func() error {
			_, err := app.RequestGitSync()
			return err
		}},
		{name: "enrollment token", run: func() error {
			_, err := app.CreateEnrollmentToken(EnrollmentTokenRequest{Fleet: "production", TTLSeconds: 3600})
			return err
		}},
		{name: "set Label", run: func() error {
			_, err := app.SetEndpointLabel(EndpointLabelSetRequest{EndpointID: "endpoint-alpha", Key: "site", Value: "berlin"})
			return err
		}},
		{name: "remove Label", run: func() error {
			_, err := app.RemoveEndpointLabel(EndpointLabelRemoveRequest{EndpointID: "endpoint-alpha", Key: "site"})
			return err
		}},
		{name: "Endpoint upgrade", run: func() error {
			_, err := app.RequestEndpointAgentUpgrade(EndpointUpgradeRequest{EndpointID: "endpoint-alpha", Version: "v2.2.0"})
			return err
		}},
		{name: "Fleet upgrade", run: func() error {
			_, err := app.RequestFleetAgentUpgrade(FleetUpgradeRequest{Fleet: "production", Version: "v2.2.0"})
			return err
		}},
		{name: "diagnostic collection", run: func() error {
			_, err := app.RequestDiagnosticCollection(DiagnosticCollectionRequest{
				EndpointID: "endpoint-alpha",
				Collectors: []string{"system_info"},
				Since:      "2026-03-03T05:05:07Z",
				Until:      "2026-03-04T05:05:07Z",
			})
			return err
		}},
		{name: "Endpoint removal", run: func() error {
			_, err := app.RemoveEndpoint(EndpointRemovalRequest{EndpointID: "endpoint-alpha", Confirmation: "endpoint-alpha"})
			return err
		}},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertForbiddenActionFailure(t, test.run())
			if after := app.sessions.Snapshot(); !reflect.DeepEqual(after, before) {
				t.Fatalf("session changed after forbidden %s\n before: %#v\n  after: %#v", test.name, before, after)
			}
			if got := actionRequests.Load(); got != int32(index+1) {
				t.Fatalf("forbidden requests after %s = %d, want %d", test.name, got, index+1)
			}
		})
	}
}

func newCrossActionAuthorizationTestApp(t *testing.T) (*App, *atomic.Int32) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	var actionRequests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-cross-action-read-only","roles":["read_only"]}`))
			return
		}
		actionRequests.Add(1)
		http.Error(response, authorizationResponseCanary, http.StatusForbidden)
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
		"operator-cross-action-read-only",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Read only", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect cross-action read-only Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	return app, &actionRequests
}

func newAuthorizationTestSession(t *testing.T) (*SessionManager, *atomic.Int32, *atomic.Int32, string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	var identityRequests atomic.Int32
	var actionRequests atomic.Int32

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if got := request.TLS.PeerCertificates[0].Subject.CommonName; got != "operator-read-only" {
			http.Error(response, "unexpected Operator identity", http.StatusUnauthorized)
			return
		}

		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			identityRequests.Add(1)
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-read-only","roles":["read_only"]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/git-sync":
			actionRequests.Add(1)
			http.Error(response, authorizationResponseCanary, http.StatusForbidden)
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
		"operator-read-only",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Read only", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect controlled read-only Operator: %v", err)
	}
	return manager, &identityRequests, &actionRequests, stateDir
}

func assertForbiddenActionFailure(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	var failure *ActionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("forbidden action error = %T %v, want *ActionFailure", err, err)
	}
	if failure.Kind != ActionForbidden {
		t.Fatalf("forbidden action kind = %q, want %q", failure.Kind, ActionForbidden)
	}
	encoded, marshalErr := json.Marshal(failure)
	if marshalErr != nil {
		t.Fatalf("encode safe action failure: %v", marshalErr)
	}
	safeText := failure.Error() + " " + string(encoded)
	if !strings.Contains(strings.ToLower(safeText), "not authorized") {
		t.Fatalf("forbidden action guidance is not authorization-specific: %s", safeText)
	}
	for _, value := range append(forbidden, authorizationResponseCanary, "BEGIN PRIVATE KEY", "BEGIN CERTIFICATE", "bootstrap") {
		if value != "" && strings.Contains(safeText, value) {
			t.Errorf("forbidden action failure disclosed %q: %s", value, safeText)
		}
	}
}
