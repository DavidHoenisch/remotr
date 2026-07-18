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
	keyFileVersion       = 2
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

// HistoricalKeyProvider resolves a retained key identity for decryption only.
type HistoricalKeyProvider interface {
	KeyProvider
	LoadByID(context.Context, string, string) (KeyMaterial, error)
}

// RotatingKeyProvider changes the active identity while retaining every
// historical identity still needed by transaction envelopes.
type RotatingKeyProvider interface {
	HistoricalKeyProvider
	Rotate(context.Context, string) (KeyMaterial, error)
}

func defaultKeyProvider() KeyProvider {
	tpm := NewTPM2ToolsKeyProvider(TPM2ToolsOptions{})
	return &CapabilityKeyProvider{Capability: tpm, TPM: tpm, Root: RootKeyProvider{}}
}

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
		class, found, existingErr := existingProtectionClass(root)
		if existingErr != nil {
			return KeyMaterial{}, fmt.Errorf("%w: determine existing protection: %w", ErrKeyProtectionUnavailable, existingErr)
		}
		if !found {
			supported, capabilityErr := p.Capability.Supported(ctx)
			if capabilityErr != nil {
				return KeyMaterial{}, fmt.Errorf("%w: determine TPM support: %w", ErrKeyProtectionUnavailable, capabilityErr)
			}
			class = ProtectionRootFile
			if supported {
				class = ProtectionTPMSealed
			}
		}
		selection = keySelectionFile{Version: keySelectionVersion, Class: class}
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

func (p *CapabilityKeyProvider) LoadByID(ctx context.Context, root, id string) (KeyMaterial, error) {
	provider, class, err := p.selectedProvider(root)
	if err != nil {
		return KeyMaterial{}, err
	}
	historical, ok := provider.(HistoricalKeyProvider)
	if !ok {
		return KeyMaterial{}, fmt.Errorf("%w: selected %s provider has no historical key resolver", ErrKeyProtectionUnavailable, class)
	}
	material, err := historical.LoadByID(ctx, root, id)
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("%w: load historical %s key %q: %w", ErrKeyProtectionUnavailable, class, id, err)
	}
	return material, nil
}

func (p *CapabilityKeyProvider) Rotate(ctx context.Context, root string) (KeyMaterial, error) {
	provider, class, err := p.selectedProvider(root)
	if err != nil {
		return KeyMaterial{}, err
	}
	rotating, ok := provider.(RotatingKeyProvider)
	if !ok {
		return KeyMaterial{}, fmt.Errorf("%w: selected %s provider does not support rotation", ErrKeyProtectionUnavailable, class)
	}
	material, err := rotating.Rotate(ctx, root)
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("%w: rotate selected %s provider: %w", ErrKeyProtectionUnavailable, class, err)
	}
	return material, nil
}

func (p *CapabilityKeyProvider) selectedProvider(root string) (KeyProvider, ProtectionClass, error) {
	if p == nil || p.TPM == nil || p.Root == nil {
		return nil, "", errors.New("rollback capability key provider is incomplete")
	}
	selection, err := readKeySelection(root)
	if err != nil {
		return nil, "", fmt.Errorf("%w: read protection selection: %w", ErrKeyProtectionUnavailable, err)
	}
	if selection.Class == ProtectionTPMSealed {
		return p.TPM, selection.Class, nil
	}
	return p.Root, selection.Class, nil
}

func existingProtectionClass(root string) (ProtectionClass, bool, error) {
	rootExists, err := fileExists(filepath.Join(root, rootKeyFilename))
	if err != nil {
		return "", false, err
	}
	tpmExists, err := fileExists(filepath.Join(root, tpmKeyFilename))
	if err != nil {
		return "", false, err
	}
	if rootExists && tpmExists {
		return "", false, errors.New("both root-file and TPM-sealed rollback keys exist without a protection selection")
	}
	if rootExists {
		return ProtectionRootFile, true, nil
	}
	if tpmExists {
		return ProtectionTPMSealed, true, nil
	}
	return "", false, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
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

type rootKeyFileV1 struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Key     []byte `json:"key"`
}

type rootKeyEntry struct {
	ID  string `json:"id"`
	Key []byte `json:"key"`
}

type rootKeyring struct {
	Version  int            `json:"version"`
	ActiveID string         `json:"activeKeyId"`
	Keys     []rootKeyEntry `json:"keys"`
}

// RootKeyProvider persists an AES-256 key in a versioned root-only file.
// Random is injectable for deterministic provider tests.
type RootKeyProvider struct {
	Random io.Reader
}

func (p RootKeyProvider) LoadOrCreate(_ context.Context, root string) (KeyMaterial, error) {
	path := filepath.Join(root, rootKeyFilename)
	keyring, err := p.loadRootKeyring(path)
	if err == nil {
		if err := os.Chmod(path, 0o600); err != nil {
			return KeyMaterial{}, err
		}
		return rootKeyMaterial(keyring, keyring.ActiveID)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return KeyMaterial{}, err
	}
	entry, err := p.newRootKeyEntry()
	if err != nil {
		return KeyMaterial{}, err
	}
	keyring = rootKeyring{Version: keyFileVersion, ActiveID: entry.ID, Keys: []rootKeyEntry{entry}}
	if err := writeJSONDurable(path, keyring); err != nil {
		clear(entry.Key)
		return KeyMaterial{}, err
	}
	return KeyMaterial{ID: entry.ID, Key: entry.Key, Protection: ProtectionRootFile}, nil
}

func (p RootKeyProvider) LoadByID(_ context.Context, root, id string) (KeyMaterial, error) {
	if !validKeyID(id) {
		return KeyMaterial{}, errors.New("historical root key identity is invalid")
	}
	keyring, err := p.loadRootKeyring(filepath.Join(root, rootKeyFilename))
	if err != nil {
		return KeyMaterial{}, err
	}
	return rootKeyMaterial(keyring, id)
}

func (p RootKeyProvider) Rotate(_ context.Context, root string) (KeyMaterial, error) {
	path := filepath.Join(root, rootKeyFilename)
	keyring, err := p.loadRootKeyring(path)
	if err != nil {
		return KeyMaterial{}, err
	}
	entry, err := p.newRootKeyEntry()
	if err != nil {
		return KeyMaterial{}, err
	}
	for _, existing := range keyring.Keys {
		if existing.ID == entry.ID {
			clear(entry.Key)
			return KeyMaterial{}, errors.New("rotated root key identity collided with retained history")
		}
	}
	keyring.ActiveID = entry.ID
	keyring.Keys = append(keyring.Keys, entry)
	if err := writeJSONDurable(path, keyring); err != nil {
		clear(entry.Key)
		return KeyMaterial{}, err
	}
	return KeyMaterial{ID: entry.ID, Key: entry.Key, Protection: ProtectionRootFile}, nil
}

func (p RootKeyProvider) loadRootKeyring(path string) (rootKeyring, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rootKeyring{}, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return rootKeyring{}, err
	}
	if len(raw) == 32 && raw[0] != '{' {
		id, err := newKeyID("root-v2", p.random())
		if err != nil {
			return rootKeyring{}, err
		}
		keyring := rootKeyring{
			Version: keyFileVersion, ActiveID: id,
			Keys: []rootKeyEntry{{ID: id, Key: append([]byte(nil), raw...)}},
		}
		if err := writeJSONDurable(path, keyring); err != nil {
			return rootKeyring{}, err
		}
		return keyring, nil
	}
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &version); err != nil {
		return rootKeyring{}, errors.New("rollback root key file is malformed")
	}
	if version.Version == 1 {
		var legacy rootKeyFileV1
		if err := decodeStrictJSON(raw, &legacy); err != nil || !validKeyID(legacy.ID) || len(legacy.Key) != 32 {
			clear(legacy.Key)
			return rootKeyring{}, errors.New("rollback root key file has an invalid version 1 key")
		}
		keyring := rootKeyring{
			Version: keyFileVersion, ActiveID: legacy.ID,
			Keys: []rootKeyEntry{{ID: legacy.ID, Key: legacy.Key}},
		}
		if err := writeJSONDurable(path, keyring); err != nil {
			return rootKeyring{}, err
		}
		return keyring, nil
	}
	var keyring rootKeyring
	if err := decodeStrictJSON(raw, &keyring); err != nil {
		return rootKeyring{}, errors.New("rollback root key file is malformed")
	}
	if err := validateRootKeyring(keyring); err != nil {
		return rootKeyring{}, err
	}
	return keyring, nil
}

func (p RootKeyProvider) newRootKeyEntry() (rootKeyEntry, error) {
	reader := p.random()
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return rootKeyEntry{}, err
	}
	id, err := newKeyID("root-v2", reader)
	if err != nil {
		clear(key)
		return rootKeyEntry{}, err
	}
	return rootKeyEntry{ID: id, Key: key}, nil
}

func validateRootKeyring(keyring rootKeyring) error {
	if keyring.Version != keyFileVersion || !validKeyID(keyring.ActiveID) || len(keyring.Keys) == 0 {
		return errors.New("rollback root keyring has an unsupported version or invalid active key")
	}
	seen := make(map[string]struct{}, len(keyring.Keys))
	activeFound := false
	for _, entry := range keyring.Keys {
		if !validKeyID(entry.ID) || len(entry.Key) != 32 {
			return errors.New("rollback root keyring contains an invalid key")
		}
		if _, exists := seen[entry.ID]; exists {
			return errors.New("rollback root keyring contains a duplicate identity")
		}
		seen[entry.ID] = struct{}{}
		activeFound = activeFound || entry.ID == keyring.ActiveID
	}
	if !activeFound {
		return errors.New("rollback root keyring active identity is missing")
	}
	return nil
}

func rootKeyMaterial(keyring rootKeyring, id string) (KeyMaterial, error) {
	for _, entry := range keyring.Keys {
		if entry.ID == id {
			return KeyMaterial{
				ID: entry.ID, Key: append([]byte(nil), entry.Key...), Protection: ProtectionRootFile,
			}, nil
		}
	}
	return KeyMaterial{}, os.ErrNotExist
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
