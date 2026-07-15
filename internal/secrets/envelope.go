package secrets

import (
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
)

// ScopeMetadata is authenticated alongside a stored secret version. It may be
// indexed and audited because it contains no secret material.
type ScopeMetadata struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Fleet      string `json:"fleet,omitempty"`
	EndpointID string `json:"endpointId,omitempty"`
}

// EncryptedRecord is the stable application-encrypted persistence format.
// The external KEK itself is deliberately absent.
type EncryptedRecord struct {
	FormatVersion int           `json:"formatVersion"`
	Algorithm     string        `json:"algorithm"`
	KEKID         string        `json:"kekId"`
	Scope         ScopeMetadata `json:"scope"`
	Ciphertext    []byte        `json:"ciphertext"`
	CipherNonce   []byte        `json:"cipherNonce"`
	WrappedDEK    []byte        `json:"wrappedDek"`
	WrapNonce     []byte        `json:"wrapNonce"`
	Fingerprint   string        `json:"fingerprint"`
}

// Clone returns a deep copy safe for mutation in persistence and validation
// code without aliasing cryptographic byte slices.
func (r EncryptedRecord) Clone() EncryptedRecord {
	r.Ciphertext = append([]byte(nil), r.Ciphertext...)
	r.CipherNonce = append([]byte(nil), r.CipherNonce...)
	r.WrappedDEK = append([]byte(nil), r.WrappedDEK...)
	r.WrapNonce = append([]byte(nil), r.WrapNonce...)
	return r
}

// Envelope encrypts each value under a fresh DEK and wraps that DEK with the
// active externally supplied KEK.
type Envelope struct {
	keyring *Keyring
	random  io.Reader
}

func NewEnvelope(keyring *Keyring) (*Envelope, error) {
	if keyring == nil {
		return nil, fmt.Errorf("external KEK keyring is required")
	}
	if _, _, err := keyring.activeKey(); err != nil {
		return nil, err
	}
	return &Envelope{keyring: keyring, random: rand.Reader}, nil
}

func (e *Envelope) Encrypt(scope ScopeMetadata, plaintext []byte) (EncryptedRecord, error) {
	if err := validateScope(scope); err != nil {
		return EncryptedRecord{}, err
	}
	if len(plaintext) == 0 || len(plaintext) > MaxMaterialBytes {
		return EncryptedRecord{}, fmt.Errorf("secret material is empty or exceeds %d bytes", MaxMaterialBytes)
	}
	kekID, kek, err := e.keyring.activeKey()
	if err != nil {
		return EncryptedRecord{}, err
	}
	record := EncryptedRecord{
		FormatVersion: EnvelopeFormatVersion,
		Algorithm:     AlgorithmAES256GCM,
		KEKID:         kekID,
		Scope:         scope,
	}
	aad, err := recordAAD(record)
	if err != nil {
		return EncryptedRecord{}, err
	}
	dek := make([]byte, dekBytes)
	if _, err := io.ReadFull(e.random, dek); err != nil {
		return EncryptedRecord{}, fmt.Errorf("generate data-encryption key: %w", err)
	}
	defer clear(dek)

	record.Ciphertext, record.CipherNonce, err = sealAESGCM(dek, plaintext, aad, e.random)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("encrypt secret record: %w", err)
	}
	record.WrappedDEK, record.WrapNonce, err = sealAESGCM(kek, dek, aad, e.random)
	if err != nil {
		return EncryptedRecord{}, fmt.Errorf("wrap data-encryption key: %w", err)
	}
	fingerprint := sha256.Sum256(record.Ciphertext)
	record.Fingerprint = "sha256:" + hex.EncodeToString(fingerprint[:])
	return record, nil
}

func (e *Envelope) Decrypt(record EncryptedRecord) ([]byte, error) {
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	kek, ok := e.keyring.key(record.KEKID)
	if !ok {
		return nil, fmt.Errorf("external KEK %q is unavailable", record.KEKID)
	}
	aad, err := recordAAD(record)
	if err != nil {
		return nil, err
	}
	dek, err := openAESGCM(kek, record.WrappedDEK, record.WrapNonce, aad)
	if err != nil {
		return nil, fmt.Errorf("unwrap data-encryption key: authentication failed")
	}
	defer clear(dek)
	if len(dek) != dekBytes {
		return nil, fmt.Errorf("wrapped data-encryption key has invalid length")
	}
	plaintext, err := openAESGCM(dek, record.Ciphertext, record.CipherNonce, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret record: authentication failed")
	}
	if len(plaintext) == 0 || len(plaintext) > MaxMaterialBytes {
		clear(plaintext)
		return nil, fmt.Errorf("decrypted secret material violates size bounds")
	}
	return plaintext, nil
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

func recordAAD(record EncryptedRecord) ([]byte, error) {
	return json.Marshal(struct {
		FormatVersion int           `json:"formatVersion"`
		Algorithm     string        `json:"algorithm"`
		KEKID         string        `json:"kekId"`
		Scope         ScopeMetadata `json:"scope"`
	}{record.FormatVersion, record.Algorithm, record.KEKID, record.Scope})
}

func validateRecord(record EncryptedRecord) error {
	if record.FormatVersion != EnvelopeFormatVersion {
		return fmt.Errorf("unsupported secret envelope format %d", record.FormatVersion)
	}
	if record.Algorithm != AlgorithmAES256GCM {
		return fmt.Errorf("unsupported secret envelope algorithm")
	}
	if strings.TrimSpace(record.KEKID) == "" || strings.TrimSpace(record.KEKID) != record.KEKID {
		return fmt.Errorf("secret envelope KEK identifier is invalid")
	}
	if err := validateScope(record.Scope); err != nil {
		return err
	}
	if len(record.Ciphertext) < 16 || len(record.Ciphertext) > MaxMaterialBytes+16 || len(record.WrappedDEK) != dekBytes+16 || len(record.CipherNonce) != 12 || len(record.WrapNonce) != 12 {
		return fmt.Errorf("secret envelope cryptographic fields are invalid")
	}
	fingerprint := sha256.Sum256(record.Ciphertext)
	expectedFingerprint := "sha256:" + hex.EncodeToString(fingerprint[:])
	if subtle.ConstantTimeCompare([]byte(record.Fingerprint), []byte(expectedFingerprint)) != 1 {
		return fmt.Errorf("secret envelope fingerprint does not match ciphertext")
	}
	return nil
}

func validateScope(scope ScopeMetadata) error {
	for label, value := range map[string]string{"name": scope.Name, "version": scope.Version} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("secret scope %s is invalid", label)
		}
	}
	if len(scope.Fleet) > 256 || len(scope.EndpointID) > 256 || strings.ContainsAny(scope.Fleet+scope.EndpointID, "\x00\r\n") {
		return fmt.Errorf("secret scope exceeds bounds")
	}
	return nil
}
