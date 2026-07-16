package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const deploymentTokenCanary = "deployment-token-canary-view-once"

type deploymentTokenParityServerState struct {
	mu             sync.Mutex
	createRequests []deploymentTokenHTTPCreateRequest
	revokeLabels   []string
	created        bool
	revoked        bool
	failCreate     bool
	forbid         bool
}

func TestDeploymentTokenParityProtectsOneTimeSecretAndUsesServerMetadata(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(originalLogOutput) })

	app, profile, stateDir, settingsDir, saveDirectory, serverState, clipboardWrites := newDeploymentTokenParityTestApp(t)

	listed, err := app.ListDeploymentTokens()
	if err != nil {
		t.Fatalf("list deployment tokens: %v", err)
	}
	if len(listed) != 1 || listed[0].Label != "nightly" || listed[0].Status != "active" {
		t.Fatalf("initial deployment tokens = %#v, want active nightly metadata", listed)
	}
	shown, err := app.LoadDeploymentToken("nightly")
	if err != nil {
		t.Fatalf("show deployment token: %v", err)
	}
	if shown.ID != "deployment-nightly" || shown.CreatedAt != "2032-03-01T05:05:07Z" || shown.LastUsedAt != "2032-03-03T05:05:07Z" {
		t.Fatalf("shown deployment token = %#v, want exact server metadata", shown)
	}

	for name, request := range map[string]DeploymentTokenCreateRequest{
		"empty label":   {Fleet: "production", TTLSeconds: 3600},
		"unknown Fleet": {Label: "prod-laptops", Fleet: "unknown", TTLSeconds: 3600},
		"zero TTL":      {Label: "prod-laptops", Fleet: "production"},
		"unsafe label":  {Label: "../prod", Fleet: "production", TTLSeconds: 3600},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := app.CreateDeploymentToken(request); err == nil {
				t.Fatalf("CreateDeploymentToken(%#v) succeeded", request)
			}
		})
	}
	serverState.mu.Lock()
	if len(serverState.createRequests) != 0 {
		t.Fatalf("invalid create input reached server: %#v", serverState.createRequests)
	}
	serverState.mu.Unlock()

	created, err := app.CreateDeploymentToken(DeploymentTokenCreateRequest{
		Label:      "prod-laptops",
		Fleet:      "production",
		TTLSeconds: 24 * 60 * 60,
	})
	if err != nil {
		t.Fatalf("create deployment token: %v", err)
	}
	if created.Token != deploymentTokenCanary || created.Metadata.Label != "prod-laptops" || created.Metadata.Fleet != "production" || created.Metadata.Status != "active" {
		t.Fatalf("created deployment token = %#v", created)
	}
	serverState.mu.Lock()
	createRequests := slices.Clone(serverState.createRequests)
	serverState.mu.Unlock()
	if len(createRequests) != 1 || createRequests[0].Label != "prod-laptops" || createRequests[0].Fleet != "production" || createRequests[0].TTLSeconds != 24*60*60 {
		t.Fatalf("create requests = %#v", createRequests)
	}
	if len(*clipboardWrites) != 0 {
		t.Fatalf("clipboard changed before explicit copy: %v", *clipboardWrites)
	}
	assertPathsExcludeCanary(t, deploymentTokenCanary, stateDir, settingsDir)

	if err := app.CopyDeploymentToken(); err != nil {
		t.Fatalf("copy deployment token: %v", err)
	}
	if !slices.Equal(*clipboardWrites, []string{deploymentTokenCanary}) {
		t.Fatalf("clipboard writes = %v", *clipboardWrites)
	}
	saved, err := app.SaveDeploymentToken("prod-laptops")
	if err != nil {
		t.Fatalf("save deployment token: %v", err)
	}
	wantSavedPath := filepath.Join(saveDirectory, "prod-laptops.token")
	if saved.Status != "saved" || saved.Path != wantSavedPath || saved.SizeBytes != int64(len(deploymentTokenCanary)+1) {
		t.Fatalf("saved deployment token = %#v", saved)
	}
	content, err := os.ReadFile(wantSavedPath)
	if err != nil {
		t.Fatalf("read protected token file: %v", err)
	}
	if string(content) != deploymentTokenCanary+"\n" {
		t.Fatalf("protected token file = %q", content)
	}
	info, err := os.Stat(wantSavedPath)
	if err != nil {
		t.Fatalf("stat protected token file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("protected token mode = %o, want 600", info.Mode().Perm())
	}

	app.ClearDeploymentToken()
	if err := app.CopyDeploymentToken(); err == nil {
		t.Fatal("copy after clear succeeded")
	}
	if _, err := app.SaveDeploymentToken("prod-laptops"); err == nil {
		t.Fatal("save after clear succeeded")
	}
	assertPathsExcludeCanary(t, deploymentTokenCanary, stateDir, settingsDir)

	serverState.mu.Lock()
	serverState.failCreate = true
	serverState.mu.Unlock()
	if _, err := app.CreateDeploymentToken(DeploymentTokenCreateRequest{Label: "failure-token", Fleet: "production", TTLSeconds: 3600}); err == nil {
		t.Fatal("server create failure returned nil error")
	} else if strings.Contains(err.Error(), deploymentTokenCanary) {
		t.Fatalf("server failure disclosed deployment token canary: %v", err)
	}
	serverState.mu.Lock()
	serverState.failCreate = false
	serverState.mu.Unlock()

	if _, err := app.RevokeDeploymentToken(DeploymentTokenRevokeRequest{Label: "prod-laptops", Confirmation: "PROD-LAPTOPS"}); err == nil {
		t.Fatal("case-insensitive revoke confirmation succeeded")
	}
	serverState.mu.Lock()
	if len(serverState.revokeLabels) != 0 {
		t.Fatalf("invalid revoke reached server: %v", serverState.revokeLabels)
	}
	serverState.mu.Unlock()

	revoked, err := app.RevokeDeploymentToken(DeploymentTokenRevokeRequest{Label: "prod-laptops", Confirmation: "prod-laptops"})
	if err != nil {
		t.Fatalf("revoke deployment token: %v", err)
	}
	if revoked.Label != "prod-laptops" || revoked.Status != "revoked" || revoked.RevokedAt != "2032-03-04T05:07:07Z" {
		t.Fatalf("revoked deployment token = %#v, want server-authoritative revocation", revoked)
	}
	serverState.mu.Lock()
	revokeLabels := slices.Clone(serverState.revokeLabels)
	serverState.mu.Unlock()
	if !slices.Equal(revokeLabels, []string{"prod-laptops"}) {
		t.Fatalf("revoke labels = %v", revokeLabels)
	}

	serverState.mu.Lock()
	serverState.forbid = true
	serverState.mu.Unlock()
	_, err = app.ListDeploymentTokens()
	var forbidden *ActionFailure
	if !errors.As(err, &forbidden) || forbidden.Kind != ActionForbidden {
		t.Fatalf("unauthorized list error = %T %v, want authorization ActionFailure", err, err)
	}
	serverState.mu.Lock()
	serverState.forbid = false
	serverState.mu.Unlock()

	if _, err := app.CreateDeploymentToken(DeploymentTokenCreateRequest{Label: "switch-token", Fleet: "production", TTLSeconds: 3600}); err != nil {
		t.Fatalf("create deployment token before profile switch: %v", err)
	}
	staging := profile
	staging.Name = "Staging"
	if _, err := app.ConnectProfile(staging); err != nil {
		t.Fatalf("switch profile: %v", err)
	}
	if err := app.CopyDeploymentToken(); err == nil {
		t.Fatal("copy after profile switch succeeded")
	}
	if _, err := app.CreateDeploymentToken(DeploymentTokenCreateRequest{Label: "shutdown-token", Fleet: "production", TTLSeconds: 3600}); err != nil {
		t.Fatalf("create deployment token before shutdown: %v", err)
	}
	app.shutdown(t.Context())
	if err := app.CopyDeploymentToken(); err == nil {
		t.Fatal("copy after shutdown succeeded")
	}

	metadataJSON, err := json.Marshal([]any{listed, shown, created.Metadata, revoked})
	if err != nil {
		t.Fatalf("encode metadata views: %v", err)
	}
	if bytes.Contains(metadataJSON, []byte(deploymentTokenCanary)) {
		t.Fatalf("metadata views disclosed one-time token: %s", metadataJSON)
	}
	if strings.Contains(logOutput.String(), deploymentTokenCanary) {
		t.Fatalf("logs disclosed deployment token canary: %s", logOutput.String())
	}
}

func newDeploymentTokenParityTestApp(t *testing.T) (*App, ConnectionProfile, string, string, string, *deploymentTokenParityServerState, *[]string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &deploymentTokenParityServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		serverState.mu.Lock()
		forbidden := serverState.forbid
		serverState.mu.Unlock()
		if forbidden && request.URL.Path != "/v1/admin/me" {
			http.Error(response, "forbidden", http.StatusForbidden)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-deployment","roles":["global_admin"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/fleets":
			_, _ = response.Write([]byte(`["production","staging"]`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/deployment-tokens":
			items := []string{deploymentTokenJSON("nightly", false)}
			serverState.mu.Lock()
			if serverState.created {
				items = append(items, deploymentTokenJSON("prod-laptops", serverState.revoked))
			}
			serverState.mu.Unlock()
			_, _ = response.Write([]byte("[" + strings.Join(items, ",") + "]"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/deployment-tokens":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read deployment token request: %v", err)
				http.Error(response, "read failed", http.StatusInternalServerError)
				return
			}
			var payload deploymentTokenHTTPCreateRequest
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode deployment token request: %v", err)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			serverState.mu.Lock()
			serverState.createRequests = append(serverState.createRequests, payload)
			serverState.created = true
			failCreate := serverState.failCreate
			serverState.mu.Unlock()
			if failCreate {
				http.Error(response, deploymentTokenCanary, http.StatusInternalServerError)
				return
			}
			_, _ = response.Write([]byte(`{"token":"` + deploymentTokenCanary + `","label":"` + payload.Label + `","fleet":"` + payload.Fleet + `","expires_at":"2032-03-05T05:05:07Z"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/deployment-tokens/nightly":
			_, _ = response.Write([]byte(deploymentTokenJSON("nightly", false)))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/deployment-tokens/prod-laptops":
			serverState.mu.Lock()
			created, revoked := serverState.created, serverState.revoked
			serverState.mu.Unlock()
			if !created {
				http.NotFound(response, request)
				return
			}
			_, _ = response.Write([]byte(deploymentTokenJSON("prod-laptops", revoked)))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/admin/deployment-tokens/prod-laptops":
			serverState.mu.Lock()
			serverState.revokeLabels = append(serverState.revokeLabels, "prod-laptops")
			serverState.revoked = true
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
		Time:         func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-deployment", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect deployment-token Operator: %v", err)
	}
	saveDirectory := t.TempDir()
	app := NewApp("test", WithClipboardWriter(func(_ context.Context, text string) error {
		return nil
	}), WithDeploymentTokenSaveDialog(func(_ context.Context, request DeploymentTokenSaveDialogRequest) (string, error) {
		return filepath.Join(saveDirectory, request.SuggestedName), nil
	}))
	clipboardWrites := []string{}
	app.writeClipboard = func(_ context.Context, text string) error {
		clipboardWrites = append(clipboardWrites, text)
		return nil
	}
	app.sessions = manager
	app.deploymentTokens.now = func() time.Time { return connectionTestTime }
	settingsDir := t.TempDir()
	app.profiles = NewProfileService(filepath.Join(settingsDir, "desktop-profiles.json"), filepath.Join(settingsDir, "operator-config.yaml"))
	return app, profile, stateDir, settingsDir, saveDirectory, serverState, &clipboardWrites
}

func deploymentTokenJSON(label string, revoked bool) string {
	id := "deployment-" + label
	lastUsed := `,"last_used_at":"2032-03-03T05:05:07Z"`
	if label == "prod-laptops" {
		lastUsed = ""
	}
	revokedAt := ""
	if revoked {
		revokedAt = `,"revoked_at":"2032-03-04T05:07:07Z"`
	}
	return `{"id":"` + id + `","label":"` + label + `","fleet":"production","created_at":"2032-03-01T05:05:07Z","expires_at":"2032-03-05T05:05:07Z"` + lastUsed + revokedAt + `}`
}

type deploymentTokenHTTPCreateRequest struct {
	Label      string `json:"label"`
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttl_seconds"`
}
