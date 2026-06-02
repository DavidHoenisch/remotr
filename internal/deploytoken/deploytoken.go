// Package deploytoken generates and verifies long-lived deployment enrollment tokens.
// Raw tokens are shown once at creation; only bcrypt hashes are persisted.
package deploytoken

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const secretBytes = 32

var (
	ErrInvalidLabel  = errors.New("invalid deployment token label")
	ErrInvalidFormat = errors.New("invalid deployment token format")
)

// ValidateLabel checks a human-readable deployment token identifier.
func ValidateLabel(label string) error {
	if label == "" {
		return ErrInvalidLabel
	}
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			continue
		default:
			return ErrInvalidLabel
		}
	}
	return nil
}

// Issue creates a new deployment token. The returned string is the only copy of the secret.
func Issue() (full string, id uuid.UUID, err error) {
	id, err = uuid.NewRandom()
	if err != nil {
		return "", uuid.Nil, err
	}
	secret := make([]byte, secretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", uuid.Nil, err
	}
	return Format(id, hex.EncodeToString(secret)), id, nil
}

// Format builds the wire token from an id and hex-encoded secret.
func Format(id uuid.UUID, secretHex string) string {
	return id.String() + "." + secretHex
}

// Parse splits a presented token into id and secret.
func Parse(presented string) (uuid.UUID, string, error) {
	presented = strings.TrimSpace(presented)
	dot := strings.LastIndex(presented, ".")
	if dot <= 0 || dot >= len(presented)-1 {
		return uuid.Nil, "", ErrInvalidFormat
	}
	id, err := uuid.Parse(presented[:dot])
	if err != nil {
		return uuid.Nil, "", ErrInvalidFormat
	}
	secret := presented[dot+1:]
	if len(secret) != secretBytes*2 {
		return uuid.Nil, "", ErrInvalidFormat
	}
	if _, err := hex.DecodeString(secret); err != nil {
		return uuid.Nil, "", ErrInvalidFormat
	}
	return id, secret, nil
}

// HashSecret returns a bcrypt hash of the token secret for storage.
func HashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash deployment token secret: %w", err)
	}
	return string(hash), nil
}

// VerifySecret compares a presented secret against a stored bcrypt hash.
func VerifySecret(storedHash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(secret)) == nil
}
