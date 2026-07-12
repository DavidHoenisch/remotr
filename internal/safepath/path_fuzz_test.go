package safepath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzReadUnderRootCannotEscape(f *testing.F) {
	f.Add("allowed")
	f.Add("../outside")
	f.Add("..")

	f.Fuzz(func(t *testing.T, element string) {
		if len(element) > 256 || strings.ContainsRune(element, '\x00') {
			return
		}
		root := t.TempDir()
		outside := filepath.Join(filepath.Dir(root), "outside")
		if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(outside) })
		if element == "allowed" {
			if err := os.WriteFile(filepath.Join(root, element), []byte("inside"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		got, _ := ReadUnderRoot(root, element)
		if string(got) == "outside-secret" {
			t.Fatalf("path %q escaped root", element)
		}
	})
}
