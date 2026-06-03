package secrets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func TestReadFileRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("abc\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := secrets.ReadFileRef("file:" + path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestReadFileRef_rejectsRelative(t *testing.T) {
	_, err := secrets.ReadFileRef("file:relative/token")
	if err == nil {
		t.Fatal("expected error")
	}
}
