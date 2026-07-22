package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	EnvelopeFormatVersion = 1
	AlgorithmAES256GCM    = "AES-256-GCM"
	dekBytes              = 32
	MaxWrappedDEKBytes    = 64 << 10
	MaxWrapMetadataBytes  = 64 << 10
)

// ScopeMetadata is authenticated alongside a stored secret version. It may be
// indexed and audited because it contains no secret material.
type ScopeMetadata struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Scope      Scope  `json:"scope,omitempty"`
	Fleet      string `json:"fleet,omitempty"`
	EndpointID string `json:"endpointId,omitempty"`
}

// EncryptedRecord is the stable application-encrypted persistence format.
// The external KEK itself is deliberately absent.
type EncryptedRecord struct {
	FormatVersion int           `json:"formatVersion"`
	Algorithm     string        `json:"algorithm"`
	KEKProvider   string        `json:"kekProvider"`
	KEKID         string        `json:"kekId"`
	WrapAlgorithm string        `json:"wrapAlgorithm"`
	Scope         ScopeMetadata `json:"scope"`
	Ciphertext    []byte        `json:"ciphertext"`
	CipherNonce   []byte        `json:"cipherNonce"`
	WrappedDEK    []byte        `json:"wrappedDek"`
	WrapMetadata  []byte        `json:"wrapMetadata,omitempty"`
	Fingerprint   string        `json:"fingerprint"`
}

// Validate checks the stable encrypted persistence format without decrypting
// or exposing secret material.
func (r EncryptedRecord) Validate() error { return validateRecord(r) }

// Clone returns a deep copy safe for mutation in persistence and validation
// code without aliasing cryptographic byte slices.
func (r EncryptedRecord) Clone() EncryptedRecord {
	r.Ciphertext = append([]byte(nil), r.Ciphertext...)
	r.CipherNonce = append([]byte(nil), r.CipherNonce...)
	r.WrappedDEK = append([]byte(nil), r.WrappedDEK...)
	r.WrapMetadata = append([]byte(nil), r.WrapMetadata...)
	return r
}

// Envelope encrypts each value under a fresh DEK and wraps that DEK with the
// active externally supplied KEK.
type Envelope struct {
	provider KeyEncryptionProvider
	random   io.Reader
}

func NewEnvelope(provider KeyEncryptionProvider) (*Envelope, error) {
	if provider == nil {
		return nil, fmt.Errorf("key-encryption provider is required")
	}
	if strings.TrimSpace(provider.ProviderID()) == "" || strings.TrimSpace(provider.ProviderID()) != provider.ProviderID() {
		return nil, fmt.Errorf("key-encryption provider identifier is invalid")
	}
	if _, err := provider.ActiveKeyID(context.Background()); err != nil {
		return nil, err
	}
	return &Envelope{provider: provider, random: rand.Reader}, nil
}

func (e *Envelope) Encrypt(scope ScopeMetadata, plaintext []byte) (EncryptedRecord, error) {
	return e.EncryptContext(context.Background(), scope, plaintext)
}

func (e *Envelope) EncryptContext(ctx context.Context, scope ScopeMetadata, plaintext []byte) (EncryptedRecord, error) {
	if err := validateScope(scope); err != nil {
		return EncryptedRecord{}, err
	}
	if len(plaintext) == 0 || len(plaintext) > MaxMaterialBytes {
		return EncryptedRecord{}, fmt.Errorf("secret material is empty or exceeds %d bytes", MaxMaterialBytes)
	}
	record := EncryptedRecord{
		FormatVersion: EnvelopeFormatVersion,
		Algorithm:     AlgorithmAES256GCM,
		Scope:         scope,
	}
	cipherAAD, err := recordCipherAAD(record)
	if err != nil {
		return EncryptedRecord{}, err
	}
	dek := make([]byte, dekBytes)
	if _, err := io.ReadFull(e.random, dek); err != nil {
		return EncryptedRecord{}, fmt.Errorf("generate data-encryption key: %w", err)
	}
	defer clear(dek)

	record.Ciphertext, record.CipherNonce, err = sealAESGCM(dek, plaintext, cipherAAD, e.random)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("encrypt secret record: %w", err)
	}
	wrapped, err := e.provider.WrapDEK(ctx, dek, cipherAAD)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("wrap data-encryption key: %w", err)
	}
	if err := validateWrappedKey(wrapped); err != nil {
		return EncryptedRecord{}, err
	}
	record.KEKProvider = wrapped.ProviderID
	record.KEKID = wrapped.KeyID
	record.WrapAlgorithm = wrapped.Algorithm
	record.WrappedDEK = append([]byte(nil), wrapped.Ciphertext...)
	record.WrapMetadata = append([]byte(nil), wrapped.Metadata...)
	fingerprint := sha256.Sum256(record.Ciphertext)
	record.Fingerprint = "sha256:" + hex.EncodeToString(fingerprint[:])
	return record, nil
}

func (e *Envelope) Decrypt(record EncryptedRecord) ([]byte, error) {
	return e.DecryptContext(context.Background(), record)
}

func (e *Envelope) DecryptContext(ctx context.Context, record EncryptedRecord) ([]byte, error) {
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	cipherAAD, err := recordCipherAAD(record)
	if err != nil {
		return nil, err
	}
	dek, err := e.provider.UnwrapDEK(ctx, wrappedKeyFromRecord(record), cipherAAD)
	if err != nil {
		return nil, fmt.Errorf("unwrap data-encryption key for secret %q version %q using %s/%s: %w", record.Scope.Name, record.Scope.Version, record.KEKProvider, record.KEKID, err)
	}
	defer clear(dek)
	if len(dek) != dekBytes {
		return nil, fmt.Errorf("wrapped data-encryption key has invalid length")
	}
	plaintext, err := openAESGCM(dek, record.Ciphertext, record.CipherNonce, cipherAAD)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret record: authentication failed")
	}
	if len(plaintext) == 0 || len(plaintext) > MaxMaterialBytes {
		clear(plaintext)
		return nil, fmt.Errorf("decrypted secret material violates size bounds")
	}
	return plaintext, nil
}

// Rewrap migrates a stored DEK to the active KEK without decrypting and
// re-encrypting the secret ciphertext.
func (e *Envelope) Rewrap(record EncryptedRecord) (EncryptedRecord, error) {
	return e.RewrapContext(context.Background(), record)
}

func (e *Envelope) RewrapContext(ctx context.Context, record EncryptedRecord) (EncryptedRecord, error) {
	if err := validateRecord(record); err != nil {
		return EncryptedRecord{}, err
	}
	cipherAAD, err := recordCipherAAD(record)
	if err != nil {
		return EncryptedRecord{}, err
	}
	dek, err := e.provider.UnwrapDEK(ctx, wrappedKeyFromRecord(record), cipherAAD)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("unwrap data-encryption key for secret %q version %q using %s/%s: %w", record.Scope.Name, record.Scope.Version, record.KEKProvider, record.KEKID, err)
	}
	defer clear(dek)
	if len(dek) != dekBytes {
		return EncryptedRecord{}, fmt.Errorf("wrapped data-encryption key has invalid length")
	}
	rewrapped := record.Clone()
	wrapped, err := e.provider.WrapDEK(ctx, dek, cipherAAD)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("rewrap data-encryption key: %w", err)
	}
	if err := validateWrappedKey(wrapped); err != nil {
		return EncryptedRecord{}, err
	}
	rewrapped.KEKProvider = wrapped.ProviderID
	rewrapped.KEKID = wrapped.KeyID
	rewrapped.WrapAlgorithm = wrapped.Algorithm
	rewrapped.WrappedDEK = append([]byte(nil), wrapped.Ciphertext...)
	rewrapped.WrapMetadata = append([]byte(nil), wrapped.Metadata...)
	return rewrapped, nil
}

func sealAESGCM(key, plaintext, aad []byte, random io.Reader) ([]byte, []byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func openAESGCM(key, ciphertext, nonce, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() || len(ciphertext) < gcm.Overhead() {
		return nil, errors.New("invalid AES-GCM record")
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256 key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func recordCipherAAD(record EncryptedRecord) ([]byte, error) {
	return json.Marshal(struct {
		FormatVersion int           `json:"formatVersion"`
		Algorithm     string        `json:"algorithm"`
		Scope         ScopeMetadata `json:"scope"`
	}{record.FormatVersion, record.Algorithm, record.Scope})
}

func validateRecord(record EncryptedRecord) error {
	if record.FormatVersion != EnvelopeFormatVersion {
		return fmt.Errorf("unsupported secret envelope format %d", record.FormatVersion)
	}
	if record.Algorithm != AlgorithmAES256GCM {
		return fmt.Errorf("unsupported secret envelope algorithm")
	}
	if err := validateWrappedKey(wrappedKeyFromRecord(record)); err != nil {
		return err
	}
	if err := validateScope(record.Scope); err != nil {
		return err
	}
	if len(record.Ciphertext) < 16 || len(record.Ciphertext) > MaxMaterialBytes+16 || len(record.CipherNonce) != 12 {
		return fmt.Errorf("secret envelope cryptographic fields are invalid")
	}
	fingerprint := sha256.Sum256(record.Ciphertext)
	expectedFingerprint := "sha256:" + hex.EncodeToString(fingerprint[:])
	if subtle.ConstantTimeCompare([]byte(record.Fingerprint), []byte(expectedFingerprint)) != 1 {
		return fmt.Errorf("secret envelope fingerprint does not match ciphertext")
	}
	return nil
}

func wrappedKeyFromRecord(record EncryptedRecord) WrappedKey {
	return WrappedKey{
		ProviderID: record.KEKProvider,
		KeyID:      record.KEKID,
		Algorithm:  record.WrapAlgorithm,
		Ciphertext: record.WrappedDEK,
		Metadata:   record.WrapMetadata,
	}
}

func validateWrappedKey(wrapped WrappedKey) error {
	for label, value := range map[string]string{"provider": wrapped.ProviderID, "key identifier": wrapped.KeyID, "algorithm": wrapped.Algorithm} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("secret envelope wrap %s is invalid", label)
		}
	}
	if len(wrapped.Ciphertext) == 0 || len(wrapped.Ciphertext) > MaxWrappedDEKBytes || len(wrapped.Metadata) > MaxWrapMetadataBytes {
		return fmt.Errorf("secret envelope wrapped-key fields are invalid")
	}
	return nil
}

func validateScope(scope ScopeMetadata) error {
	for label, value := range map[string]string{"name": scope.Name, "version": scope.Version} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("secret scope %s is invalid", label)
		}
	}
	_, _, _, err := normalizeScope(scope.Scope, scope.Fleet, scope.EndpointID, true)
	return err
}
