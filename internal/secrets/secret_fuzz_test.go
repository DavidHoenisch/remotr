package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzReadFileRefTrimsOnlySecretValue(f *testing.F) {
	f.Add("token", " secret-value\n")
	f.Add("token", "\n")

	f.Fuzz(func(t *testing.T, name, value string) {
		if len(name) > 128 || len(value) > 1<<16 || name == "" || name == "." || strings.TrimSpace(name) != name || filepath.Clean(name) != name || strings.ContainsAny(name, "/\\\x00") || strings.Contains(name, "..") {
			return
		}
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := ReadFileRef("file:" + path)
		want := strings.TrimSpace(value)
		if want == "" {
			if err == nil {
				t.Fatal("empty secret was accepted")
			}
			return
		}
		if err != nil || got != want {
			t.Fatalf("secret = %q, err = %v, want %q", got, err, want)
		}
	})
}
