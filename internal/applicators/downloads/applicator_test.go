package downloads_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mockCurl(content []byte) *executil.MockRunner {
	m := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			fmt.Sprintf("curl %v", []string{"-fsSL", "https://example.com/bin"}): {
				Stdout: content,
			},
		},
	}
	return m
}

func TestApplicator_State_missing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "agent")
	a := downloads.New(models.DownloadResource{
		Name: "agent",
		URL:  "https://example.com/bin",
		Dest: dest,
	}, nil)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected missing file drift")
	}
}

func TestApplicator_State_modeDrift(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	if err := os.WriteFile(dest, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	a := downloads.New(models.DownloadResource{
		Name: "bin",
		URL:  "https://example.com/bin",
		Dest: dest,
		Mode: []int{0755},
	}, nil)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected mode drift")
	}
}

func TestApplicator_State_checksumDrift(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	content := []byte("old")
	if err := os.WriteFile(dest, content, 0644); err != nil {
		t.Fatal(err)
	}
	want := sha256Hex([]byte("new"))
	a := downloads.New(models.DownloadResource{
		Name:     "bin",
		URL:      "https://example.com/bin",
		Dest:     dest,
		Checksum: "sha256:" + want,
	}, nil)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected checksum drift")
	}
}

func TestApplicator_State_met(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	content := []byte("payload")
	if err := os.WriteFile(dest, content, 0755); err != nil {
		t.Fatal(err)
	}
	a := downloads.New(models.DownloadResource{
		Name:     "bin",
		URL:      "https://example.com/bin",
		Dest:     dest,
		Mode:     []int{0755},
		Checksum: "sha256:" + sha256Hex(content),
	}, nil)
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected state met")
	}
}

func TestApplicator_Apply_downloadsAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "nested", "bin")
	content := []byte("#!/bin/sh\necho hi\n")
	mock := mockCurl(content)
	a := downloads.New(models.DownloadResource{
		Name: "bin",
		URL:  "https://example.com/bin",
		Dest: dest,
		Mode: []int{0755},
	}, mock)

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if len(mock.Calls) != 1 || mock.Calls[0].Name != "curl" {
		t.Fatalf("calls = %+v", mock.Calls)
	}
}

func TestApplicator_Apply_checksumMismatch(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	mock := mockCurl([]byte("wrong"))
	a := downloads.New(models.DownloadResource{
		Name:     "bin",
		URL:      "https://example.com/bin",
		Dest:     dest,
		Checksum: "sha256:" + sha256Hex([]byte("expected")),
	}, mock)
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestApplicator_ApplyResult_notifySystemd(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "unit.conf")
	content := []byte("x")
	mock := mockCurl(content)
	mock.Next[fmt.Sprintf("systemctl %v", []string{"try-restart", "mysvc.service"})] = executil.MockResult{}
	a := downloads.New(models.DownloadResource{
		Name:          "cfg",
		URL:           "https://example.com/bin",
		Dest:          dest,
		NotifySystemd: "mysvc.service",
	}, mock)
	result := a.ApplyResult(context.Background())
	if result.Status != executor.Changed || !reflect.DeepEqual(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationTryRestart, Target: "mysvc.service"}}) {
		t.Fatalf("result = %+v", result)
	}
	foundCurl := false
	for _, c := range mock.Calls {
		if c.Name == "curl" {
			foundCurl = true
		}
		if c.Name == "systemctl" {
			t.Fatalf("activation executed inside provider: %+v", c)
		}
	}
	if !foundCurl {
		t.Fatalf("calls = %+v", mock.Calls)
	}
}

func TestApplicator_Apply_notifySystemd_fallbackReload(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "unit.conf")
	content := []byte("x")
	mock := mockCurl(content)
	mock.Next[fmt.Sprintf("systemctl %v", []string{"try-restart", "mysvc.service"})] = executil.MockResult{Err: fmt.Errorf("failed")}
	mock.Next[fmt.Sprintf("systemctl %v", []string{"daemon-reload"})] = executil.MockResult{}
	mock.Next[fmt.Sprintf("systemctl %v", []string{"restart", "mysvc.service"})] = executil.MockResult{}
	a := downloads.New(models.DownloadResource{
		Name:          "cfg",
		URL:           "https://example.com/bin",
		Dest:          dest,
		NotifySystemd: "mysvc.service",
	}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestApplicator_Apply_reloadExec(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "audit.rules")
	content := []byte("-w /etc/passwd -p wa\n")
	mock := mockCurl(content)
	mock.Next[fmt.Sprintf("augenrules %v", []string{"--load"})] = executil.MockResult{}
	a := downloads.New(models.DownloadResource{
		Name:          "audit-rules",
		URL:           "https://example.com/bin",
		Dest:          dest,
		ReloadExec:    []string{"systemctl", "reload", "auditd.service"},
		NotifySystemd: "auditd.service",
	}, mock)
	result := a.ApplyResult(context.Background())
	if !reflect.DeepEqual(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationReload, Target: "auditd.service"}}) {
		t.Fatalf("activation = %+v", result.Activation)
	}
	for _, c := range mock.Calls {
		if c.Name == "systemctl" {
			t.Fatalf("reloadExec must be queued, got systemctl call: %+v", c)
		}
	}
}

func TestApplicator_Apply_reloadExecError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "audit.rules")
	mock := mockCurl([]byte("x"))
	mock.Next[fmt.Sprintf("augenrules %v", []string{"--load"})] = executil.MockResult{Err: fmt.Errorf("load failed")}
	a := downloads.New(models.DownloadResource{
		Name:       "audit-rules",
		URL:        "https://example.com/bin",
		Dest:       dest,
		ReloadExec: []string{"augenrules", "--load"},
	}, mock)
	result := a.ApplyResult(context.Background())
	if result.Status != executor.Changed {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplicator_Revert_restoresBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	original := []byte("original\n")
	if err := os.WriteFile(dest, original, 0644); err != nil {
		t.Fatal(err)
	}
	content := []byte("new\n")
	mock := mockCurl(content)
	a := downloads.New(models.DownloadResource{
		Name:     "bin",
		URL:      "https://example.com/bin",
		Dest:     dest,
		Checksum: "sha256:" + sha256Hex(content),
	}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("reverted = %q", data)
	}
	if _, err := os.Stat(dest + ".remotr.bak"); !os.IsNotExist(err) {
		t.Fatal("expected backup removed")
	}
}

func TestApplicator_ProtectedRollbackSurvivesRestartWithoutAdjacentBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	if err := os.WriteFile(dest, []byte("original\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	rollbackRoot := filepath.Join(dir, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	first := downloads.New(models.DownloadResource{
		Name: "bin", URL: "https://example.com/bin", Dest: dest,
		Checksum: "sha256:" + sha256Hex([]byte("new\n")), Mode: []int{0o755},
	}, mockCurl([]byte("new\n")))
	if err := first.ConfigureRollback(store, "base/bin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := first.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".remotr.bak"); !os.IsNotExist(err) {
		t.Fatalf("generic adjacent backup exists: %v", err)
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := downloads.New(first.Download, mockCurl([]byte("new\n")))
	if err := restarted.ConfigureRollback(restartedStore, "base/bin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "original\n" {
		t.Fatalf("restored destination = %q, %v", got, err)
	}
	if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("restored mode = %v, %v", info.Mode().Perm(), err)
	}
	if _, met := restarted.State(ctx); met {
		t.Fatal("second Check after rollback is compliant; want drifted")
	}
}

func TestApplicator_Revert_removesWhenNoBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	mock := mockCurl([]byte("only"))
	a := downloads.New(models.DownloadResource{
		Name: "bin",
		URL:  "https://example.com/bin",
		Dest: dest,
	}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("expected dest removed")
	}
}

func TestApplicator_destValidation(t *testing.T) {
	a := downloads.New(models.DownloadResource{
		Name: "x",
		URL:  "https://example.com/x",
		Dest: "relative/path",
	}, nil)
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestApplicator_signatureMismatchPreservesActiveFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "policy")
	if err := os.WriteFile(dest, []byte("active"), 0o600); err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := downloads.New(models.DownloadResource{
		Name: "policy", URL: "https://example.com/bin", Dest: dest,
		Checksum:      "sha256:" + sha256Hex([]byte("untrusted")),
		Signature:     base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
		TrustedSigner: base64.StdEncoding.EncodeToString(pub),
	}, mockCurl([]byte("untrusted")))
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected signature rejection")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "active" {
		t.Fatalf("active file changed: %q, %v", data, err)
	}
}

func TestApplicator_absentRemovesDestinationWithoutFetch(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "obsolete")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := mockCurl([]byte("unused"))
	a := downloads.New(models.DownloadResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "obsolete", Dest: dest}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination remains: %v", err)
	}
	if len(mock.Calls) != 0 {
		t.Fatalf("unexpected fetch: %+v", mock.Calls)
	}
}
