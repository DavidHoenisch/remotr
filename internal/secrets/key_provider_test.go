package secrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"testing"
)

func TestStaticKeyEncryptionProviderContract(t *testing.T) {
	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": bytes.Repeat([]byte{0x81}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	runKeyEncryptionProviderContract(t, keyring)
}

func TestKMSStyleProviderContractAndCrossProviderRewrap(t *testing.T) {
	static, err := NewKeyring("static-old", map[string][]byte{"static-old": bytes.Repeat([]byte{0xa1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	staticEnvelope, err := NewEnvelope(static)
	if err != nil {
		t.Fatal(err)
	}
	record, err := staticEnvelope.Encrypt(ScopeMetadata{Name: "service/api-key", Version: "5", Scope: ScopeGlobal}, []byte("provider-canary"))
	if err != nil {
		t.Fatal(err)
	}
	kms := &kmsStyleTestProvider{keyID: "projects/p/locations/l/keyRings/r/cryptoKeys/k/versions/2", key: bytes.Repeat([]byte{0xb1}, 32)}
	runKeyEncryptionProviderContract(t, kms)

	router, err := NewKeyEncryptionRouter(kms, static)
	if err != nil {
		t.Fatal(err)
	}
	migratingEnvelope, err := NewEnvelope(router)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := migratingEnvelope.Rewrap(record)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.KEKProvider != "test-kms" || migrated.KEKID != kms.keyID || migrated.WrapAlgorithm != "test-kms-opaque-v1" || len(migrated.WrapMetadata) == 0 {
		t.Fatalf("migrated wrapping metadata = %#v", migrated)
	}
	if !bytes.Equal(migrated.Ciphertext, record.Ciphertext) || !bytes.Equal(migrated.CipherNonce, record.CipherNonce) {
		t.Fatal("cross-provider rewrap changed secret ciphertext")
	}
	plaintext, err := migratingEnvelope.Decrypt(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "provider-canary" {
		t.Fatalf("plaintext = %q", plaintext)
	}
}

func runKeyEncryptionProviderContract(t *testing.T, provider KeyEncryptionProvider) {
	t.Helper()
	ctx := context.Background()
	keyID, err := provider.ActiveKeyID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ProviderID() == "" || keyID == "" {
		t.Fatalf("provider ID=%q key ID=%q", provider.ProviderID(), keyID)
	}
	dek := bytes.Repeat([]byte{0x91}, 32)
	aad := []byte(`{"secret":"database/password","version":"3"}`)
	wrapped, err := provider.WrapDEK(ctx, dek, aad)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.ProviderID != provider.ProviderID() || wrapped.KeyID != keyID || wrapped.Algorithm == "" || len(wrapped.Ciphertext) == 0 {
		t.Fatalf("wrapped DEK metadata = %#v", wrapped)
	}
	got, err := provider.UnwrapDEK(ctx, wrapped, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatalf("unwrapped DEK = %x", got)
	}

	tampered := wrapped.Clone()
	tampered.Ciphertext[0] ^= 0xff
	if _, err := provider.UnwrapDEK(ctx, tampered, aad); err == nil {
		t.Fatal("tampered wrapped DEK was accepted")
	}
	if _, err := provider.UnwrapDEK(ctx, wrapped, []byte("wrong authenticated scope")); err == nil {
		t.Fatal("wrapped DEK was accepted for the wrong authenticated scope")
	}
	available, err := provider.KeyAvailable(ctx, provider.ProviderID(), keyID)
	if err != nil || !available {
		t.Fatalf("active key available=%v err=%v", available, err)
	}
}

type kmsStyleTestProvider struct {
	keyID string
	key   []byte
}

func (*kmsStyleTestProvider) ProviderID() string { return "test-kms" }

func (p *kmsStyleTestProvider) ActiveKeyID(context.Context) (string, error) { return p.keyID, nil }

func (p *kmsStyleTestProvider) WrapDEK(_ context.Context, dek, aad []byte) (WrappedKey, error) {
	wrapped := WrappedKey{ProviderID: p.ProviderID(), KeyID: p.keyID, Algorithm: "test-kms-opaque-v1"}
	var err error
	wrapped.Ciphertext, wrapped.Metadata, err = sealAESGCM(p.key, dek, kmsStyleAAD(wrapped, aad), rand.Reader)
	return wrapped, err
}

func (p *kmsStyleTestProvider) UnwrapDEK(_ context.Context, wrapped WrappedKey, aad []byte) ([]byte, error) {
	if wrapped.ProviderID != p.ProviderID() || wrapped.KeyID != p.keyID || wrapped.Algorithm != "test-kms-opaque-v1" {
		return nil, fmt.Errorf("unsupported KMS wrapping identity")
	}
	return openAESGCM(p.key, wrapped.Ciphertext, wrapped.Metadata, kmsStyleAAD(wrapped, aad))
}

func (p *kmsStyleTestProvider) KeyAvailable(_ context.Context, providerID, keyID string) (bool, error) {
	return providerID == p.ProviderID() && keyID == p.keyID, nil
}

func kmsStyleAAD(wrapped WrappedKey, aad []byte) []byte {
	return append([]byte(wrapped.ProviderID+"\x00"+wrapped.KeyID+"\x00"+wrapped.Algorithm+"\x00"), aad...)
}
