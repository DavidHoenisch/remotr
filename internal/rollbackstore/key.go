package rollbackstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	rootKeyFilename      = "rollback.key"
	keySelectionFilename = "rollback-protection.json"
	keyFileVersion       = 1
	keySelectionVersion  = 1

	// RootCompromiseLimitation is the explicit safety limitation attached to
	// the root-file fallback report.
	RootCompromiseLimitation = "does not protect against endpoint-root compromise"
)

var ErrKeyProtectionUnavailable = errors.New("rollback key protection is unavailable")

// ProtectionClass identifies how endpoint-local rollback key material is
// protected at rest.
type ProtectionClass string

const (
	ProtectionTPMSealed ProtectionClass = "tpm-sealed"
	ProtectionRootFile  ProtectionClass = "root-file-reduced"
)

func (class ProtectionClass) valid() bool {
	return class == ProtectionTPMSealed || class == ProtectionRootFile
}

// KeyMaterial is returned only to the rollback store. ID and Protection are
// safe metadata; Key must never be copied into diagnostics or reports.
type KeyMaterial struct {
	ID         string
	Key        []byte
	Protection ProtectionClass
}

// ProtectionReport is the safe, key-free view suitable for capability and
// diagnostic reporting.
type ProtectionReport struct {
	Class             ProtectionClass `json:"class"`
	KeyID             string          `json:"keyId"`
	ReducedProtection bool            `json:"reducedProtection"`
	Limitation        string          `json:"limitation,omitempty"`
}

// KeyProvider supplies one versioned endpoint-local encryption key.
type KeyProvider interface {
	LoadOrCreate(context.Context, string) (KeyMaterial, error)
}

func defaultKeyProvider() KeyProvider { return RootKeyProvider{} }

// TPMCapability distinguishes unsupported endpoints from a selected provider
// that is temporarily failing. Only the former may select the root fallback.
type TPMCapability interface {
	Supported(context.Context) (bool, error)
}

// CapabilityKeyProvider persists its protection-class choice before loading
// key material. A selected TPM failure therefore remains blocking and cannot
// silently fall back on the current or a later startup.
type CapabilityKeyProvider struct {
	Capability TPMCapability
	TPM        KeyProvider
	Root       KeyProvider
}

type keySelectionFile struct {
	Version int             `json:"version"`
	Class   ProtectionClass `json:"class"`
}

func (p *CapabilityKeyProvider) LoadOrCreate(ctx context.Context, root string) (KeyMaterial, error) {
	if p == nil || p.Capability == nil || p.TPM == nil || p.Root == nil {
		return KeyMaterial{}, errors.New("rollback capability key provider is incomplete")
	}
	selection, err := readKeySelection(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return KeyMaterial{}, fmt.Errorf("%w: read protection selection: %w", ErrKeyProtectionUnavailable, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		supported, capabilityErr := p.Capability.Supported(ctx)
		if capabilityErr != nil {
			return KeyMaterial{}, fmt.Errorf("%w: determine TPM support: %w", ErrKeyProtectionUnavailable, capabilityErr)
		}
		selection = keySelectionFile{Version: keySelectionVersion, Class: ProtectionRootFile}
		if supported {
			selection.Class = ProtectionTPMSealed
		}
		if err := writeJSONDurable(filepath.Join(root, keySelectionFilename), selection); err != nil {
			return KeyMaterial{}, fmt.Errorf("%w: persist protection selection: %w", ErrKeyProtectionUnavailable, err)
		}
	}
	provider := p.Root
	if selection.Class == ProtectionTPMSealed {
		provider = p.TPM
	}
	material, err := provider.LoadOrCreate(ctx, root)
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("%w: selected %s provider: %w", ErrKeyProtectionUnavailable, selection.Class, err)
	}
	if material.Protection != selection.Class {
		clear(material.Key)
		return KeyMaterial{}, fmt.Errorf("%w: selected %s provider reported %s", ErrKeyProtectionUnavailable, selection.Class, material.Protection)
	}
	return material, nil
}

func readKeySelection(root string) (keySelectionFile, error) {
	raw, err := os.ReadFile(filepath.Join(root, keySelectionFilename))
	if err != nil {
		return keySelectionFile{}, err
	}
	var selection keySelectionFile
	if err := decodeStrictJSON(raw, &selection); err != nil {
		return keySelectionFile{}, err
	}
	if selection.Version != keySelectionVersion || !selection.Class.valid() {
		return keySelectionFile{}, errors.New("rollback protection selection has an unsupported version or class")
	}
	return selection, nil
}

type rootKeyFile struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Key     []byte `json:"key"`
}

// RootKeyProvider persists an AES-256 key in a versioned root-only file.
// Random is injectable for deterministic provider tests.
type RootKeyProvider struct {
	Random io.Reader
}

func (p RootKeyProvider) LoadOrCreate(_ context.Context, root string) (KeyMaterial, error) {
	path := filepath.Join(root, rootKeyFilename)
	raw, err := os.ReadFile(path)
	if err == nil {
		if len(raw) == 32 && raw[0] != '{' {
			return p.migrateLegacy(path, raw)
		}
		var stored rootKeyFile
		if err := decodeStrictJSON(raw, &stored); err != nil {
			return KeyMaterial{}, errors.New("rollback root key file is malformed")
		}
		if stored.Version != keyFileVersion || !validKeyID(stored.ID) || len(stored.Key) != 32 {
			clear(stored.Key)
			return KeyMaterial{}, errors.New("rollback root key file has an unsupported version or invalid key")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			clear(stored.Key)
			return KeyMaterial{}, err
		}
		return KeyMaterial{ID: stored.ID, Key: stored.Key, Protection: ProtectionRootFile}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return KeyMaterial{}, err
	}
	reader := p.random()
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return KeyMaterial{}, err
	}
	id, err := newKeyID("root-v1", reader)
	if err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	if err := writeJSONDurable(path, rootKeyFile{Version: keyFileVersion, ID: id, Key: key}); err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	return KeyMaterial{ID: id, Key: key, Protection: ProtectionRootFile}, nil
}

func (p RootKeyProvider) migrateLegacy(path string, legacy []byte) (KeyMaterial, error) {
	key := append([]byte(nil), legacy...)
	id, err := newKeyID("root-v1", p.random())
	if err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	if err := writeJSONDurable(path, rootKeyFile{Version: keyFileVersion, ID: id, Key: key}); err != nil {
		clear(key)
		return KeyMaterial{}, err
	}
	return KeyMaterial{ID: id, Key: key, Protection: ProtectionRootFile}, nil
}

func (p RootKeyProvider) random() io.Reader {
	if p.Random != nil {
		return p.Random
	}
	return rand.Reader
}

func newKeyID(prefix string, random io.Reader) (string, error) {
	id := make([]byte, 16)
	if _, err := io.ReadFull(random, id); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(id), nil
}

func validKeyID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9', char == '-', char == '_', char == '.', char == ':':
		default:
			return false
		}
	}
	return true
}

func writeJSONDurable(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeAtomic(path, raw, 0o600); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func decodeStrictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("JSON value has trailing content")
		}
		return err
	}
	return nil
}
