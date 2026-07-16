package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const enrollmentTokenCanary = "enroll-token-canary-never-persist"

type enrollmentTokenServerState struct {
	createRequests atomic.Int32
	failCreate     atomic.Bool
	mu             sync.Mutex
	requests       []enrollmentTokenHTTPPayload
}

type enrollmentTokenHTTPPayload struct {
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

func TestEnrollmentTokenValidationCopyAndSecretLifecycle(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(originalLogOutput) })

	app, profile, stateDir, settingsDir, serverState, clipboardWrites := newEnrollmentTokenTestApp(t)

	for name, request := range map[string]EnrollmentTokenRequest{
		"empty Fleet":   {Fleet: "", TTLSeconds: 3600},
		"unknown Fleet": {Fleet: "unknown", TTLSeconds: 3600},
		"zero TTL":      {Fleet: "production", TTLSeconds: 0},
		"negative TTL":  {Fleet: "production", TTLSeconds: -1},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := app.CreateEnrollmentToken(request)
			if err == nil {
				t.Fatalf("CreateEnrollmentToken(%#v) = %#v, want validation error", request, result)
			}
			if strings.Contains(err.Error(), enrollmentTokenCanary) {
				t.Fatalf("validation error disclosed token canary: %v", err)
			}
		})
	}
	if got := serverState.createRequests.Load(); got != 0 {
		t.Fatalf("invalid input reached enrollment mutation endpoint %d time(s)", got)
	}

	result, err := app.CreateEnrollmentToken(EnrollmentTokenRequest{
		Fleet:      "production",
		TTLSeconds: 24 * 60 * 60,
	})
	if err != nil {
		t.Fatalf("create enrollment token: %v", err)
	}
	if result.Token != enrollmentTokenCanary || result.Fleet != "production" || result.ExpiresAt != "2032-03-05T05:05:07Z" {
		t.Fatalf("enrollment result = %#v", result)
	}
	if got := serverState.createRequests.Load(); got != 1 {
		t.Fatalf("create requests = %d, want 1", got)
	}
	serverState.mu.Lock()
	requests := slices.Clone(serverState.requests)
	serverState.mu.Unlock()
	if len(requests) != 1 || requests[0].Fleet != "production" || requests[0].TTLSeconds != 24*60*60 {
		t.Fatalf("enrollment requests = %#v", requests)
	}
	if len(*clipboardWrites) != 0 {
		t.Fatalf("clipboard changed before explicit copy: %v", *clipboardWrites)
	}
	if err := app.SaveProfile(profile); err != nil {
		t.Fatalf("save profile while token is transient: %v", err)
	}
	assertPathsExcludeCanary(t, enrollmentTokenCanary, stateDir, settingsDir)

	if err := app.CopyEnrollmentToken(); err != nil {
		t.Fatalf("copy enrollment token: %v", err)
	}
	if !slices.Equal(*clipboardWrites, []string{enrollmentTokenCanary}) {
		t.Fatalf("clipboard writes = %v, want one direct token copy", *clipboardWrites)
	}

	app.ClearEnrollmentToken()
	if err := app.CopyEnrollmentToken(); err == nil {
		t.Fatal("copy after clear succeeded, want cleared transient secret")
	} else if strings.Contains(err.Error(), enrollmentTokenCanary) {
		t.Fatalf("copy-after-clear error disclosed token canary: %v", err)
	}
	if strings.Contains(app.sessions.Snapshot().Local.TransientResult, enrollmentTokenCanary) {
		t.Fatal("session local state retained enrollment token canary")
	}

	if err := app.SaveProfile(profile); err != nil {
		t.Fatalf("save profile while token is cleared: %v", err)
	}
	assertPathsExcludeCanary(t, enrollmentTokenCanary, stateDir, settingsDir)

	serverState.failCreate.Store(true)
	_, err = app.CreateEnrollmentToken(EnrollmentTokenRequest{
		Fleet:      "production",
		TTLSeconds: 3600,
	})
	if err == nil {
		t.Fatal("server failure returned nil error")
	}
	if strings.Contains(err.Error(), enrollmentTokenCanary) {
		t.Fatalf("later error disclosed enrollment token canary: %v", err)
	}
	serverState.failCreate.Store(false)

	if _, err := app.CreateEnrollmentToken(EnrollmentTokenRequest{Fleet: "production", TTLSeconds: 3600}); err != nil {
		t.Fatalf("create token before profile switch: %v", err)
	}
	staging := profile
	staging.Name = "Staging"
	if _, err := app.ConnectProfile(staging); err != nil {
		t.Fatalf("switch profile: %v", err)
	}
	if err := app.CopyEnrollmentToken(); err == nil {
		t.Fatal("copy after profile switch succeeded, want cleared transient secret")
	}

	if _, err := app.CreateEnrollmentToken(EnrollmentTokenRequest{Fleet: "production", TTLSeconds: 3600}); err != nil {
		t.Fatalf("create token before shutdown: %v", err)
	}
	app.shutdown(t.Context())
	if err := app.CopyEnrollmentToken(); err == nil {
		t.Fatal("copy after shutdown succeeded, want cleared transient secret")
	}

	if !slices.Equal(*clipboardWrites, []string{enrollmentTokenCanary}) {
		t.Fatalf("clipboard retained hidden duplicate writes: %v", *clipboardWrites)
	}
	assertPathsExcludeCanary(t, enrollmentTokenCanary, stateDir, settingsDir)
	if strings.Contains(logOutput.String(), enrollmentTokenCanary) {
		t.Fatalf("logs disclosed enrollment token canary: %s", logOutput.String())
	}
}

func newEnrollmentTokenTestApp(t *testing.T) (*App, ConnectionProfile, string, string, *enrollmentTokenServerState, *[]string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	serverState := &enrollmentTokenServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-enrollment","roles":["global_admin"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/fleets":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`["production","staging"]`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/enroll-tokens":
			serverState.createRequests.Add(1)
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read enrollment request: %v", err)
				http.Error(response, "read failed", http.StatusInternalServerError)
				return
			}
			var payload enrollmentTokenHTTPPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode enrollment request: %v", err)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			serverState.mu.Lock()
			serverState.requests = append(serverState.requests, payload)
			serverState.mu.Unlock()
			if serverState.failCreate.Load() {
				http.Error(response, enrollmentTokenCanary, http.StatusInternalServerError)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"token":"` + enrollmentTokenCanary + `","fleet":"production","expires_at":"2032-03-05T05:05:07Z"}`))
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
		"operator-enrollment",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect enrollment Operator: %v", err)
	}

	clipboardWrites := []string{}
	app := NewApp("test", WithClipboardWriter(func(_ context.Context, text string) error {
		clipboardWrites = append(clipboardWrites, text)
		return nil
	}))
	app.sessions = manager
	settingsDir := t.TempDir()
	app.profiles = NewProfileService(
		filepath.Join(settingsDir, "desktop-profiles.json"),
		filepath.Join(settingsDir, "operator-config.yaml"),
	)
	return app, profile, stateDir, settingsDir, serverState, &clipboardWrites
}

func assertPathsExcludeCanary(t *testing.T, canary string, roots ...string) {
	t.Helper()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(content, []byte(canary)) {
				t.Errorf("enrollment token canary persisted in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for enrollment token canary: %v", root, err)
		}
	}
}
