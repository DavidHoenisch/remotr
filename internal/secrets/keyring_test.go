package secrets

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExternalKeyringRequiresProtectedVersionedKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret-keks.json")
	active := base64.StdEncoding.EncodeToString(make([]byte, 32))
	historical := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("h", 32)))
	data := fmt.Sprintf(`{"active":"kek-2026-07","keys":{"kek-2026-07":"%s","kek-2026-06":"%s"}}`, active, historical)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	keyring, err := LoadKeyringFile(path, WithKeyringRequiredUID(uint32(os.Getuid())))
	if err != nil {
		t.Fatal(err)
	}
	if keyring.ActiveID() != "kek-2026-07" || !keyring.Has("kek-2026-06") {
		t.Fatalf("keyring active=%q historical=%v", keyring.ActiveID(), keyring.Has("kek-2026-06"))
	}

	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyringFile(path, WithKeyringRequiredUID(uint32(os.Getuid()))); err == nil {
		t.Fatal("group-readable keyring was accepted")
	}

	link := filepath.Join(dir, "linked-keyring.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyringFile(link, WithKeyringRequiredUID(uint32(os.Getuid()))); err == nil {
		t.Fatal("symlinked keyring was accepted")
	}
}

func TestLoadKeyringJSONRejectsMissingActiveAndUnknownFields(t *testing.T) {
	validKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	tests := map[string]string{
		"missing active": fmt.Sprintf(`{"keys":{"kek-1":"%s"}}`, validKey),
		"unknown active": fmt.Sprintf(`{"active":"missing","keys":{"kek-1":"%s"}}`, validKey),
		"unknown field":  fmt.Sprintf(`{"active":"kek-1","keys":{"kek-1":"%s"},"plaintext":"no"}`, validKey),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadKeyringJSON([]byte(input)); err == nil {
				t.Fatal("invalid keyring was accepted")
			}
		})
	}
}
