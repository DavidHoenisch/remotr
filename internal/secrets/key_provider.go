package secrets

import (
	"context"
	"crypto/rand"
	"fmt"
)

const (
	StaticKeyProviderID    = "static-keyring"
	StaticKeyWrapAlgorithm = "AES-256-GCM"
)

// WrappedKey is the provider-neutral stored representation of an encrypted
// DEK. Metadata is opaque to the envelope and must never contain a plaintext
// key or secret value.
type WrappedKey struct {
	ProviderID string `json:"providerId"`
	KeyID      string `json:"keyId"`
	Algorithm  string `json:"algorithm"`
	Ciphertext []byte `json:"ciphertext"`
	Metadata   []byte `json:"metadata,omitempty"`
}

func (w WrappedKey) Clone() WrappedKey {
	w.Ciphertext = append([]byte(nil), w.Ciphertext...)
	w.Metadata = append([]byte(nil), w.Metadata...)
	return w
}

// KeyEncryptionProvider is the external key-wrapping boundary. Implementors
// may use a protected static keyring, KMS, or HSM without changing secret
// references or EncryptedRecord semantics.
type KeyEncryptionProvider interface {
	ProviderID() string
	ActiveKeyID(context.Context) (string, error)
	WrapDEK(context.Context, []byte, []byte) (WrappedKey, error)
	UnwrapDEK(context.Context, WrappedKey, []byte) ([]byte, error)
	KeyAvailable(context.Context, string, string) (bool, error)
}

var _ KeyEncryptionProvider = (*Keyring)(nil)

func (k *Keyring) ProviderID() string { return StaticKeyProviderID }

func (k *Keyring) ActiveKeyID(context.Context) (string, error) {
	id, _, err := k.activeKey()
	return id, err
}

func (k *Keyring) WrapDEK(_ context.Context, dek, aad []byte) (WrappedKey, error) {
	if len(dek) != dekBytes {
		return WrappedKey{}, fmt.Errorf("data-encryption key must be 32 bytes")
	}
	keyID, key, err := k.activeKey()
	if err != nil {
		return WrappedKey{}, err
	}
	wrapped := WrappedKey{
		ProviderID: StaticKeyProviderID,
		KeyID:      keyID,
		Algorithm:  StaticKeyWrapAlgorithm,
	}
	wrapAAD := staticKeyWrapAAD(wrapped, aad)
	wrapped.Ciphertext, wrapped.Metadata, err = sealAESGCM(key, dek, wrapAAD, rand.Reader)
	if err != nil {
		return WrappedKey{}, fmt.Errorf("wrap data-encryption key: %w", err)
	}
	return wrapped, nil
}

func (k *Keyring) UnwrapDEK(_ context.Context, wrapped WrappedKey, aad []byte) ([]byte, error) {
	if wrapped.ProviderID != StaticKeyProviderID {
		return nil, fmt.Errorf("static keyring cannot unwrap provider %q", wrapped.ProviderID)
	}
	if wrapped.Algorithm != StaticKeyWrapAlgorithm {
		return nil, fmt.Errorf("static keyring does not support wrap algorithm %q", wrapped.Algorithm)
	}
	key, ok := k.key(wrapped.KeyID)
	if !ok {
		return nil, fmt.Errorf("external KEK %q is unavailable", wrapped.KeyID)
	}
	dek, err := openAESGCM(key, wrapped.Ciphertext, wrapped.Metadata, staticKeyWrapAAD(wrapped, aad))
	if err != nil {
		return nil, fmt.Errorf("unwrap data-encryption key: authentication failed")
	}
	if len(dek) != dekBytes {
		clear(dek)
		return nil, fmt.Errorf("wrapped data-encryption key has invalid length")
	}
	return dek, nil
}

func (k *Keyring) KeyAvailable(_ context.Context, providerID, keyID string) (bool, error) {
	return providerID == StaticKeyProviderID && k.Has(keyID), nil
}

func staticKeyWrapAAD(wrapped WrappedKey, aad []byte) []byte {
	prefix := []byte("remotr-key-wrap-v1\x00" + wrapped.ProviderID + "\x00" + wrapped.KeyID + "\x00" + wrapped.Algorithm + "\x00")
	return append(prefix, aad...)
}
