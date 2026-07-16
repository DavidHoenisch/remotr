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

const desktopSecretCanary = "desktop-secret-material-canary-never-view-state"

type secretParityState struct {
	mu              sync.Mutex
	uploadBodies    [][]byte
	uploadQueries   []string
	lifecycleBodies map[string][][]byte
	dialogCalls     int
	forbid          bool
	malformed       bool
}

func TestEncryptedSecretParityProtectsMaterialAndPreservesRolloutPlanning(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(originalLogOutput) })

	material := []byte(desktopSecretCanary)
	app, state, settingsDir := newSecretParityTestApp(t, &material)

	for name, request := range map[string]SecretUploadRequest{
		"missing scope":    {Name: "wifi/office"},
		"invalid scope":    {Name: "wifi/office", ScopeType: "organization", ScopeID: "production"},
		"unknown Fleet":    {Name: "wifi/office", ScopeType: "fleet", ScopeID: "unknown"},
		"unknown endpoint": {Name: "wifi/office", ScopeType: "endpoint", ScopeID: "missing"},
		"invalid name":     {Name: "../wifi", ScopeType: "fleet", ScopeID: "production"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := app.UploadSecretVersion(request); err == nil {
				t.Fatalf("UploadSecretVersion(%#v) succeeded", request)
			}
		})
	}
	state.mu.Lock()
	if state.dialogCalls != 0 || len(state.uploadBodies) != 0 {
		t.Fatalf("invalid input reached protected input/server: dialogs=%d uploads=%d", state.dialogCalls, len(state.uploadBodies))
	}
	state.mu.Unlock()

	uploaded, err := app.UploadSecretVersion(SecretUploadRequest{
		Name: "wifi/office", ScopeType: "fleet", ScopeID: "production",
	})
	if err != nil {
		t.Fatalf("upload secret: %v", err)
	}
	if uploaded.Name != "wifi/office" || uploaded.Version != "2" || uploaded.ScopeType != "fleet" || uploaded.ScopeID != "production" || uploaded.Status != "inactive" {
		t.Fatalf("uploaded metadata = %#v", uploaded)
	}
	if !allZero(material) {
		t.Fatalf("protected material was not cleared: %q", material)
	}
	state.mu.Lock()
	uploadBodies := cloneByteSlices(state.uploadBodies)
	uploadQueries := slices.Clone(state.uploadQueries)
	state.mu.Unlock()
	if len(uploadBodies) != 1 || string(uploadBodies[0]) != desktopSecretCanary || !slices.Equal(uploadQueries, []string{"fleet=production&name=wifi%2Foffice"}) {
		t.Fatalf("upload bodies=%q queries=%v", uploadBodies, uploadQueries)
	}

	versions, err := app.ListSecretVersions("wifi/office")
	if err != nil {
		t.Fatalf("list secret versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Version != "1" || versions[1].Version != "2" || versions[0].Fingerprint == "" {
		t.Fatalf("secret versions = %#v", versions)
	}

	if _, err := app.ActivateSecretVersion(SecretLifecycleRequest{
		Name: "wifi/office", Version: "2", Confirmation: "wifi/office@2",
	}); err == nil {
		t.Fatal("activation without exact ACTIVATE scope succeeded")
	}
	activated, err := app.ActivateSecretVersion(SecretLifecycleRequest{
		Name: "wifi/office", Version: "2", Confirmation: "wifi/office@2 ACTIVATE",
	})
	if err != nil {
		t.Fatalf("activate secret: %v", err)
	}
	if activated.Status != "activation_planned" || activated.ActivationGeneration != 3 || len(activated.Rollouts) != 1 || activated.Rollouts[0].ChangeRequestID != "change-secret-2" || activated.Rollouts[0].Risk != "connectivity" {
		t.Fatalf("activated metadata = %#v", activated)
	}

	if _, err := app.RevokeSecretVersion(SecretLifecycleRequest{
		Name: "wifi/office", Version: "2", Confirmation: "WIFI/OFFICE@2 REVOKE",
	}); err == nil {
		t.Fatal("case-insensitive revocation confirmation succeeded")
	}
	revoked, err := app.RevokeSecretVersion(SecretLifecycleRequest{
		Name: "wifi/office", Version: "2", Confirmation: "wifi/office@2 REVOKE",
	})
	if err != nil {
		t.Fatalf("revoke secret: %v", err)
	}
	if revoked.Status != "revoked" || !revoked.ResolutionBlocked || revoked.EndpointCopyStatus != "rotation-or-removal-required" {
		t.Fatalf("revoked metadata = %#v", revoked)
	}

	state.mu.Lock()
	activateBodies := cloneByteSlices(state.lifecycleBodies["activate"])
	revokeBodies := cloneByteSlices(state.lifecycleBodies["revoke"])
	state.mu.Unlock()
	wantLifecycle := []byte(`{"name":"wifi/office","version":"2"}`)
	if len(activateBodies) != 1 || len(revokeBodies) != 1 || !bytes.Equal(activateBodies[0], wantLifecycle) || !bytes.Equal(revokeBodies[0], wantLifecycle) {
		t.Fatalf("lifecycle bodies activate=%q revoke=%q", activateBodies, revokeBodies)
	}

	state.mu.Lock()
	state.malformed = true
	state.mu.Unlock()
	if _, err := app.ListSecretVersions("wifi/office"); err == nil {
		t.Fatal("malformed secret metadata succeeded")
	}
	state.mu.Lock()
	state.malformed = false
	state.forbid = true
	state.mu.Unlock()
	_, err = app.ListSecretVersions("wifi/office")
	var forbidden *ActionFailure
	if !errors.As(err, &forbidden) || forbidden.Kind != ActionForbidden {
		t.Fatalf("forbidden list error = %T %v", err, err)
	}

	encoded, err := json.Marshal([]any{uploaded, versions, activated, revoked})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbiddenValue := range []string{desktopSecretCanary, "ciphertext", "wrappedDEK", "private key"} {
		if bytes.Contains(bytes.ToLower(encoded), bytes.ToLower([]byte(forbiddenValue))) {
			t.Fatalf("secret view metadata exposed %q: %s", forbiddenValue, encoded)
		}
		if strings.Contains(strings.ToLower(logOutput.String()), strings.ToLower(forbiddenValue)) {
			t.Fatalf("logs exposed %q: %s", forbiddenValue, logOutput.String())
		}
	}
	assertPathsExcludeCanary(t, desktopSecretCanary, settingsDir)
}

func TestProtectedSecretNativeInputRejectsUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected.secret")
	if err := os.WriteFile(protected, []byte(desktopSecretCanary), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := readProtectedSecretFile(protected, uint32(os.Getuid()))
	if err != nil {
		t.Fatalf("read protected file: %v", err)
	}
	if string(material) != desktopSecretCanary {
		t.Fatalf("protected material = %q", material)
	}
	zeroBytes(material)

	unsafe := filepath.Join(root, "unsafe.secret")
	if err := os.WriteFile(unsafe, []byte(desktopSecretCanary), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedSecretFile(unsafe, uint32(os.Getuid())); err == nil {
		t.Fatal("group/world-readable secret file succeeded")
	}
	symlink := filepath.Join(root, "linked.secret")
	if err := os.Symlink(protected, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedSecretFile(symlink, uint32(os.Getuid())); err == nil {
		t.Fatal("symbolic-link secret file succeeded")
	}
}

func newSecretParityTestApp(t *testing.T, material *[]byte) (*App, *secretParityState, string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	state := &secretParityState{lifecycleBodies: map[string][][]byte{}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		state.mu.Lock()
		forbidden := state.forbid
		malformed := state.malformed
		state.mu.Unlock()
		if forbidden && request.URL.Path != "/v1/admin/me" {
			http.Error(response, desktopSecretCanary, http.StatusForbidden)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-secrets","roles":["global_admin"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/fleets":
			_, _ = response.Write([]byte(`["production","staging"]`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/endpoints":
			_, _ = response.Write([]byte(`[{"id":"endpoint-1","fleet":"production"}]`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/secrets/versions":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read secret upload: %v", err)
				return
			}
			if request.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("upload content type = %q", request.Header.Get("Content-Type"))
			}
			state.mu.Lock()
			state.uploadBodies = append(state.uploadBodies, slices.Clone(body))
			state.uploadQueries = append(state.uploadQueries, request.URL.RawQuery)
			state.mu.Unlock()
			response.WriteHeader(http.StatusCreated)
			_, _ = response.Write([]byte(secretMetadataJSON(false, false, false)))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/secrets":
			if request.URL.Query().Get("name") != "wifi/office" {
				http.Error(response, "bad name", http.StatusBadRequest)
				return
			}
			if malformed {
				invalid := strings.Replace(secretMetadataJSON(false, false, false), `"fingerprint":"sha256:`+strings.Repeat("a", 64)+`"`, `"fingerprint":"invalid","ciphertext":"`+desktopSecretCanary+`"`, 1)
				_, _ = response.Write([]byte("[" + invalid + "]"))
				return
			}
			_, _ = response.Write([]byte("[" + strings.Replace(secretMetadataJSON(false, false, false), `"version":"2"`, `"version":"1"`, 1) + "," + secretMetadataJSON(false, false, false) + "]"))
		case request.Method == http.MethodPost && (request.URL.Path == "/v1/admin/secrets/activate" || request.URL.Path == "/v1/admin/secrets/revoke"):
			action := strings.TrimPrefix(request.URL.Path, "/v1/admin/secrets/")
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read secret lifecycle: %v", err)
				return
			}
			state.mu.Lock()
			state.lifecycleBodies[action] = append(state.lifecycleBodies[action], slices.Clone(body))
			state.mu.Unlock()
			_, _ = response.Write([]byte(secretMetadataJSON(action == "activate", action == "revoke", action == "activate")))
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: connectionCertPool(t, fixture.caPEM), MinVersion: tls.VersionTLS12,
		Time: func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-secrets", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Secret Operator: %v", err)
	}
	service := NewSecretService(
		func(context.Context) (string, error) {
			state.mu.Lock()
			state.dialogCalls++
			state.mu.Unlock()
			return "/native/protected.secret", nil
		},
		func(string, uint32) ([]byte, error) { return *material, nil },
		uint32(os.Getuid()),
	)
	app := NewApp("test", WithSecretService(service))
	app.sessions = manager
	settingsDir := t.TempDir()
	app.profiles = NewProfileService(filepath.Join(settingsDir, "desktop-profiles.json"), filepath.Join(settingsDir, "operator-config.yaml"))
	return app, state, settingsDir
}

func secretMetadataJSON(active, revoked, rollout bool) string {
	rollouts := "[]"
	generation := "0"
	activatedAt := ""
	if rollout {
		generation = "3"
		activatedAt = `,"activatedAt":"2032-03-04T05:06:07Z","activatedBy":"operator-secrets"`
		rollouts = `[{"fleet":"production","resourceAddress":"office/wifi","purpose":"network-credential","risk":"connectivity","effectiveHash":"sha256:` + strings.Repeat("c", 64) + `","changeRequestId":"change-secret-2"}]`
	}
	revokedFields := ""
	if revoked {
		revokedFields = `,"revokedAt":"2032-03-04T05:07:07Z","revokedBy":"operator-secrets","resolutionBlocked":true,"endpointCopyStatus":"rotation-or-removal-required"`
	}
	return `{"name":"wifi/office","version":"2","fingerprint":"sha256:` + strings.Repeat("a", 64) + `","fleet":"production","active":` + boolString(active) + `,"activationGeneration":` + generation + `,"createdAt":"2032-03-04T05:05:07Z","createdBy":"operator-secrets","revoked":` + boolString(revoked) + activatedAt + revokedFields + `,"rollouts":` + rollouts + `}`
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func cloneByteSlices(values [][]byte) [][]byte {
	cloned := make([][]byte, len(values))
	for index := range values {
		cloned[index] = slices.Clone(values[index])
	}
	return cloned
}

func allZero(value []byte) bool {
	for _, current := range value {
		if current != 0 {
			return false
		}
	}
	return true
}
