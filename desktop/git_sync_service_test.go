package main

import (
	"crypto/tls"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestGitSyncUsesOnlyAuthenticatedServerOperationWithoutRepositoryWrites(t *testing.T) {
	repository := newGitSyncSentinelRepository(t)
	before := snapshotGitSyncRepository(t, repository)
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatalf("enter sentinel repository: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	manager, requests := newGitSyncTestSession(t)
	app := NewApp("test")
	app.sessions = manager

	result, err := app.RequestGitSync()
	if err != nil {
		t.Fatalf("request Git sync: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("Git sync requests = %d, want 1", got)
	}
	if result.Status != "accepted" || result.Action != "git_sync" || result.Target != "config-repo" {
		t.Fatalf("Git sync result identity = %#v", result)
	}
	if result.ProfileName != "Production" {
		t.Fatalf("Git sync result profile = %q, want Production", result.ProfileName)
	}
	if result.Summary != "Server accepted Git sync for the Production profile." {
		t.Fatalf("Git sync summary = %q", result.Summary)
	}
	if !reflect.DeepEqual(result.AffectedEvidence, []string{"release_ref", "activity"}) {
		t.Fatalf("affected evidence = %v", result.AffectedEvidence)
	}

	after := snapshotGitSyncRepository(t, repository)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("Git sync changed the local repository\n before: %#v\n  after: %#v", before, after)
	}
}

func newGitSyncTestSession(t *testing.T) (*SessionManager, *atomic.Int32) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	var requests atomic.Int32

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"operator_id":"operator-git-sync","roles":["global_admin"]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/git-sync":
			requests.Add(1)
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write([]byte("ok"))
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
		"operator-git-sync",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		fixture.caPEM,
	)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Git sync Operator: %v", err)
	}
	return manager, &requests
}

func newGitSyncSentinelRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	for name, content := range map[string]string{
		".git/HEAD":              "ref: refs/heads/main\n",
		".git/index":             "sentinel-index\n",
		".git/refs/heads/main":   "sentinel-commit\n",
		"config/fleets/main.yml": "kind: manifest\n",
	} {
		path := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create sentinel parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write sentinel %s: %v", name, err)
		}
	}
	return repository
}

func snapshotGitSyncRepository(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = fmt.Sprintf("%04o:%s", info.Mode().Perm(), content)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot sentinel repository: %v", err)
	}
	return snapshot
}
