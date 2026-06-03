package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallBinary_replacesDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "remotr-agent")
	if err := os.WriteFile(src, []byte("new-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installBinary(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("dest = %q", got)
	}
	if _, err := os.Stat(dest + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staging file left behind: %v", err)
	}
}
