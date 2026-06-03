package downloads_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
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

func TestApplicator_Apply_notifySystemd(t *testing.T) {
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
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	foundCurl, foundRestart := false, false
	for _, c := range mock.Calls {
		if c.Name == "curl" {
			foundCurl = true
		}
		if c.Name == "systemctl" && len(c.Args) > 0 && c.Args[0] == "try-restart" {
			foundRestart = true
		}
	}
	if !foundCurl || !foundRestart {
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
