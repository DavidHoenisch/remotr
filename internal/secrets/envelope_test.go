package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnvelopeEncryptsIdenticalPlaintextWithIndependentDEKsAndNonces(t *testing.T) {
	keyring, err := NewKeyring("kek-2026-07", map[string][]byte{
		"kek-2026-07": bytes.Repeat([]byte{0x42}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	scope := ScopeMetadata{Name: "repositories/private", Version: "1", Fleet: "production"}
	plaintext := []byte("same secret value")

	first, err := envelope.Encrypt(scope, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	second, err := envelope.Encrypt(scope, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	if first.FormatVersion != EnvelopeFormatVersion || first.Algorithm != AlgorithmAES256GCM || first.KEKID != "kek-2026-07" {
		t.Fatalf("first record metadata = %#v", first)
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("identical plaintext produced identical ciphertext")
	}
	if bytes.Equal(first.CipherNonce, second.CipherNonce) {
		t.Fatal("cipher nonce was reused")
	}
	if bytes.Equal(first.WrappedDEK, second.WrappedDEK) {
		t.Fatal("fresh DEKs did not produce independent wrapped keys")
	}
	if bytes.Equal(first.WrapNonce, second.WrapNonce) {
		t.Fatal("DEK wrapping nonce was reused")
	}
	for i, record := range []EncryptedRecord{first, second} {
		got, err := envelope.Decrypt(record)
		if err != nil {
			t.Fatalf("decrypt record %d: %v", i, err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("record %d plaintext = %q", i, got)
		}
	}
	if bytes.Contains(first.Ciphertext, plaintext) || bytes.Contains(first.WrappedDEK, plaintext) {
		t.Fatal("encrypted record contains plaintext")
	}
}

func TestEnvelopeAuthenticatesRecordScopeAndRejectsMalformedRecords(t *testing.T) {
	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": bytes.Repeat([]byte{0x11}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := envelope.Encrypt(ScopeMetadata{Name: "database/password", Version: "4", EndpointID: "endpoint-1"}, []byte("canary-do-not-log"))
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*EncryptedRecord){
		"scope":      func(r *EncryptedRecord) { r.Scope.EndpointID = "endpoint-2" },
		"ciphertext": func(r *EncryptedRecord) { r.Ciphertext[0] ^= 0xff },
		"wrapped DEK": func(r *EncryptedRecord) {
			r.WrappedDEK[0] ^= 0xff
		},
		"algorithm": func(r *EncryptedRecord) { r.Algorithm = "AES-128-GCM" },
		"format":    func(r *EncryptedRecord) { r.FormatVersion++ },
		"fingerprint": func(r *EncryptedRecord) {
			r.Fingerprint = "sha256:" + strings.Repeat("0", 64)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := record.Clone()
			mutate(&tampered)
			if _, err := envelope.Decrypt(tampered); err == nil {
				t.Fatal("tampered record was accepted")
			} else if bytes.Contains([]byte(err.Error()), []byte("canary-do-not-log")) {
				t.Fatal("error exposed plaintext")
			}
		})
	}
}

func TestEnvelopeRejectsInvalidKeyAndMaterialBoundaries(t *testing.T) {
	if _, err := NewKeyring("short", map[string][]byte{"short": make([]byte, 31)}); err == nil {
		t.Fatal("31-byte KEK was accepted")
	}
	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	for name, material := range map[string][]byte{
		"empty":     nil,
		"oversized": make([]byte, MaxMaterialBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := envelope.Encrypt(ScopeMetadata{Name: "bounded", Version: "1"}, material); err == nil {
				t.Fatal("invalid material was accepted")
			}
		})
	}
}
