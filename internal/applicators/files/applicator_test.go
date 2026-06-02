package files_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_regexMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(path, []byte("PermitRootLogin no\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{
		Name:     "sshd",
		Path:     path,
		WithRegx: `(?m)^PermitRootLogin no`,
	})
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected regex match")
	}
}

func TestApplicator_contentDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{Name: "motd", Path: path, Content: "new\n"})
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected content drift")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("content = %q", data)
	}
}
