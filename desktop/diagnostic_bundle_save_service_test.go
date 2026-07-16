package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const diagnosticBundleByteCanary = "diagnostic-bundle-byte-canary\n"

type diagnosticBundleSaveServerState struct {
	detailRequests   atomic.Int32
	downloadRequests atomic.Int32
	dialogCalls      atomic.Int32
	mu               sync.Mutex
	dialogRequests   []string
	destinations     map[string]string
}

func TestReadyDiagnosticBundleIsSavedNativelyWithVerifiedMetadata(t *testing.T) {
	app, state, targetDirectory := newDiagnosticBundleSaveTestApp(t)

	result, err := app.SaveDiagnosticBundle("diagnostic-ready")
	if err != nil {
		t.Fatalf("save ready diagnostic bundle: %v", err)
	}
	wantPath := filepath.Join(targetDirectory, "diagnostic-ready.tar.gz")
	want := DiagnosticBundleSaveResult{
		Status:    "saved",
		Path:      wantPath,
		SizeBytes: int64(len(diagnosticBundleByteCanary)),
	}
	if result != want {
		t.Fatalf("save result = %#v, want %#v", result, want)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read saved diagnostic bundle: %v", err)
	}
	if string(content) != diagnosticBundleByteCanary {
		t.Fatalf("saved bundle = %q, want exact controlled bytes", content)
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat saved diagnostic bundle: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("saved bundle mode = %o, want 600", info.Mode().Perm())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode save result: %v", err)
	}
	if strings.Contains(string(encoded), diagnosticBundleByteCanary) || strings.Contains(string(encoded), "diagnosticBytes") {
		t.Fatalf("ordinary frontend result contains diagnostic bytes: %s", encoded)
	}
	if state.dialogCalls.Load() != 1 || state.downloadRequests.Load() != 1 {
		t.Fatalf("ready save dialog/download calls = %d/%d, want 1/1", state.dialogCalls.Load(), state.downloadRequests.Load())
	}
	assertNoDiagnosticTemporaryFiles(t, targetDirectory)
}

func TestDiagnosticBundleSaveRejectsNonReadyLifecycleBeforeDestination(t *testing.T) {
	app, state, targetDirectory := newDiagnosticBundleSaveTestApp(t)

	for _, test := range []struct {
		requestID string
		condition string
	}{
		{requestID: "diagnostic-pending", condition: "pending"},
		{requestID: "diagnostic-failed", condition: "failed"},
		{requestID: "diagnostic-expired", condition: "expired"},
		{requestID: "diagnostic-missing", condition: "not found"},
	} {
		t.Run(test.requestID, func(t *testing.T) {
			_, err := app.SaveDiagnosticBundle(test.requestID)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.condition) {
				t.Fatalf("save %s error = %v, want exact %q lifecycle condition", test.requestID, err, test.condition)
			}
			if _, statErr := os.Stat(filepath.Join(targetDirectory, test.requestID+".tar.gz")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("non-ready request %s created a destination: %v", test.requestID, statErr)
			}
		})
	}
	if state.dialogCalls.Load() != 0 {
		t.Fatalf("non-ready requests opened the native save dialog %d time(s), want 0", state.dialogCalls.Load())
	}
	if state.downloadRequests.Load() != 0 {
		t.Fatalf("non-ready requests downloaded %d bundle(s), want 0", state.downloadRequests.Load())
	}
	assertNoDiagnosticTemporaryFiles(t, targetDirectory)
}

func TestDiagnosticBundleSaveCleansInterruptedAndMismatchedWrites(t *testing.T) {
	app, state, targetDirectory := newDiagnosticBundleSaveTestApp(t)

	for _, requestID := range []string{
		"diagnostic-bad-size",
		"diagnostic-bad-digest",
		"diagnostic-interrupted",
	} {
		t.Run(requestID, func(t *testing.T) {
			_, err := app.SaveDiagnosticBundle(requestID)
			if err == nil {
				t.Fatalf("save %s succeeded, want verified-write failure", requestID)
			}
			if _, statErr := os.Stat(filepath.Join(targetDirectory, requestID+".tar.gz")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("failed save %s left a final destination: %v", requestID, statErr)
			}
			assertNoDiagnosticTemporaryFiles(t, targetDirectory)
		})
	}
	if state.dialogCalls.Load() != 3 || state.downloadRequests.Load() != 3 {
		t.Fatalf("failed verified saves dialog/download calls = %d/%d, want 3/3", state.dialogCalls.Load(), state.downloadRequests.Load())
	}
}

func assertNoDiagnosticTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	temporary, err := filepath.Glob(filepath.Join(directory, ".remotr-diagnostic-*.tmp"))
	if err != nil {
		t.Fatalf("inspect diagnostic temporary files: %v", err)
	}
	if len(temporary) != 0 {
		t.Fatalf("diagnostic temporary files remain: %v", temporary)
	}
}

func newDiagnosticBundleSaveTestApp(t *testing.T) (*App, *diagnosticBundleSaveServerState, string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	targetDirectory := t.TempDir()
	state := &diagnosticBundleSaveServerState{destinations: make(map[string]string)}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-diagnostic-save","roles":["global_admin"]}`))
			return
		}
		if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.Path, "/v1/admin/diagnostics/") {
			http.NotFound(response, request)
			return
		}

		suffix := strings.TrimPrefix(request.URL.Path, "/v1/admin/diagnostics/")
		if strings.HasSuffix(suffix, "/download") {
			state.downloadRequests.Add(1)
			requestID := strings.TrimSuffix(suffix, "/download")
			if requestID == "diagnostic-interrupted" {
				response.Header().Set("Content-Length", "128")
				_, _ = response.Write([]byte("partial"))
				return
			}
			_, _ = response.Write([]byte(diagnosticBundleByteCanary))
			return
		}

		state.detailRequests.Add(1)
		requestID := suffix
		if requestID == "diagnostic-missing" {
			http.NotFound(response, request)
			return
		}
		status := strings.TrimPrefix(requestID, "diagnostic-")
		if status == "bad-size" || status == "bad-digest" || status == "interrupted" {
			status = "ready"
		}
		digest := sha256.Sum256([]byte(diagnosticBundleByteCanary))
		sha := hex.EncodeToString(digest[:])
		size := int64(len(diagnosticBundleByteCanary))
		switch requestID {
		case "diagnostic-bad-size":
			size++
		case "diagnostic-bad-digest":
			sha = strings.Repeat("0", sha256.Size*2)
		case "diagnostic-interrupted":
			size = 128
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id":          requestID,
			"endpoint_id": "endpoint-alpha",
			"status":      status,
			"spec": map[string]any{
				"collectors": []string{"system_info"},
				"since":      "2026-03-03T05:05:07Z",
				"until":      "2026-03-04T05:05:07Z",
			},
			"sha256":     sha,
			"size_bytes": size,
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
		"operator-diagnostic-save",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect diagnostic save Operator: %v", err)
	}
	dialog := func(_ context.Context, suggestedName string) (string, error) {
		state.dialogCalls.Add(1)
		state.mu.Lock()
		defer state.mu.Unlock()
		state.dialogRequests = append(state.dialogRequests, suggestedName)
		requestID := strings.TrimSuffix(suggestedName, ".tar.gz")
		destination := filepath.Join(targetDirectory, requestID+".tar.gz")
		state.destinations[requestID] = destination
		return destination, nil
	}
	app := NewApp("test", WithDiagnosticBundleSaveDialog(dialog))
	app.sessions = manager
	return app, state, targetDirectory
}

func ExampleDiagnosticBundleSaveDialog() {
	var dialog DiagnosticBundleSaveDialog = func(_ context.Context, suggestedName string) (string, error) {
		return filepath.Join("/chosen", suggestedName), nil
	}
	path, _ := dialog(context.Background(), "diagnostic-42.tar.gz")
	fmt.Println(filepath.Base(path))
	// Output: diagnostic-42.tar.gz
}
