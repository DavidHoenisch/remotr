package safepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadUnderRoot_rejectsTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := ReadUnderRoot(root, "..", "etc", "passwd")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadConfigFile_requiresAbsolute(t *testing.T) {
	_, err := ReadConfigFile("relative.pem")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadUnderRoot_readsFile(t *testing.T) {
	root := t.TempDir()
	want := []byte("ok")
	dir := filepath.Join(root, "fleets", "a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "desired.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadUnderRoot(root, "fleets", "a", "desired.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q", got)
	}
}
