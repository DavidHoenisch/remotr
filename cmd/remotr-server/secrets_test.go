package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretProviderStartupFailsClosedWithoutExternalKeyring(t *testing.T) {
	getenv := func(key string) string {
		if key == "REMOTR_SECRETS_ENABLED" {
			return "true"
		}
		return ""
	}
	if _, err := loadSecretEnvelopeFromEnvironment(getenv, uint32(os.Getuid())); err == nil {
		t.Fatal("enabled Remotr provider started without an external keyring")
	} else if !strings.Contains(err.Error(), "external KEK keyring") {
		t.Fatalf("startup diagnostic = %q", err)
	}

	disabled, err := loadSecretEnvelopeFromEnvironment(func(string) string { return "" }, uint32(os.Getuid()))
	if err != nil || disabled != nil {
		t.Fatalf("disabled provider envelope=%v err=%v", disabled, err)
	}
}

func TestSecretProviderStartupLoadsProtectedKeyring(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keyring.json")
	encoded := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"active":"kek-1","keys":{"kek-1":"%s"}}`, encoded)), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"REMOTR_SECRETS_ENABLED":    "true",
		"REMOTR_SECRET_KEK_KEYRING": path,
	}
	envelope, err := loadSecretEnvelopeFromEnvironment(func(key string) string { return values[key] }, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	if envelope == nil {
		t.Fatal("protected external keyring was not loaded")
	}
}
