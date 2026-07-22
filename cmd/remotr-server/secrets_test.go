package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/secrets"
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

type restoredSecretRecords struct {
	records []secrets.EncryptedRecord
}

func (source restoredSecretRecords) ListEncryptedSecretRecords(context.Context) ([]secrets.EncryptedRecord, error) {
	return source.records, nil
}

func TestSecretProviderStartupValidatesRestoredDatabaseKeyCoverage(t *testing.T) {
	const canary = "database-restore-secret-canary"
	oldKey := bytes.Repeat([]byte{0x31}, 32)
	newKey := bytes.Repeat([]byte{0x41}, 32)
	oldKeyring, err := secrets.NewKeyring("kek-old", map[string][]byte{"kek-old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := secrets.NewEnvelope(oldKeyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := oldEnvelope.Encrypt(secrets.ScopeMetadata{Name: "database/password", Version: "1", Scope: secrets.ScopeGlobal}, []byte(canary))
	if err != nil {
		t.Fatal(err)
	}
	source := restoredSecretRecords{records: []secrets.EncryptedRecord{record}}

	newOnly, err := secrets.NewKeyring("kek-new", map[string][]byte{"kek-new": newKey})
	if err != nil {
		t.Fatal(err)
	}
	newOnlyEnvelope, err := secrets.NewEnvelope(newOnly)
	if err != nil {
		t.Fatal(err)
	}
	err = validateRestoredSecretKeyCoverage(t.Context(), source, newOnlyEnvelope)
	if err == nil || !strings.Contains(err.Error(), "1 encrypted secret version") {
		t.Fatalf("missing restored KEK startup error = %v", err)
	}
	for _, forbidden := range []string{canary, string(oldKey), string(newKey)} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("restore startup diagnostic leaked secret material: %v", err)
		}
	}

	completeKeyring, err := secrets.NewKeyring("kek-new", map[string][]byte{"kek-new": newKey, "kek-old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	completeEnvelope, err := secrets.NewEnvelope(completeKeyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRestoredSecretKeyCoverage(t.Context(), source, completeEnvelope); err != nil {
		t.Fatalf("complete restored KEK coverage = %v", err)
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
