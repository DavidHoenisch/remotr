package rollbackstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func (s *Store) migrateLegacyRecords(_ context.Context) error {
	if err := s.cleanupMigratedLegacyArtifacts(); err != nil {
		return fmt.Errorf("%w: %w", ErrRecoveryBlocked, err)
	}
	root := filepath.Join(s.root, "records")
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		if info.Name() == "metadata.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecoveryBlocked, err)
	}
	sort.Strings(paths)
	for _, metadataPath := range paths {
		if err := s.migrateLegacyRecord(metadataPath); err != nil {
			return fmt.Errorf("%w: %w", ErrRecoveryBlocked, err)
		}
	}
	return nil
}

func (s *Store) migrateLegacyRecord(metadataPath string) error {
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		return err
	}
	var meta metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	if err := normalizeMetadata(&meta); err != nil {
		return err
	}
	key := recordKey{Address: meta.Address, ArtifactDigest: meta.ArtifactDigest, Attempt: meta.Attempt}
	dir := filepath.Dir(metadataPath)
	if dir != s.recordDir(key.Address, key.ArtifactDigest, key.Attempt) {
		return errors.New("legacy rollback metadata path does not match record identity")
	}
	payloadPath := filepath.Join(dir, "payload.bin")
	ciphertext, readErr := os.ReadFile(payloadPath)
	present := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if !present && meta.Armed {
		return errors.New("armed legacy rollback record has no payload")
	}
	var payload []byte
	if present {
		payload, err = s.openLegacyPayload(meta, ciphertext)
		if err != nil {
			return err
		}
	}
	meta.Version = RecordVersion
	encoded, err := s.sealEnvelope(meta, payload, present)
	clear(payload)
	if err != nil {
		return err
	}
	if err := s.writeEnvelopeAtomic(dir, encoded); err != nil {
		return err
	}
	return removeLegacyArtifacts(dir)
}

func (s *Store) openLegacyPayload(meta metadata, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(meta.Nonce) != gcm.NonceSize() || len(ciphertext) < gcm.Overhead() {
		return nil, errors.New("legacy rollback payload is malformed")
	}
	payload, err := gcm.Open(nil, meta.Nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("legacy rollback payload authentication failed")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != meta.Checksum {
		clear(payload)
		return nil, errors.New("legacy rollback payload checksum mismatch")
	}
	return payload, nil
}

func (s *Store) cleanupMigratedLegacyArtifacts() error {
	root := filepath.Join(s.root, "records")
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || !info.IsDir() {
			return walkErr
		}
		envelopePath := filepath.Join(path, envelopeFilename)
		if raw, err := os.ReadFile(envelopePath); err == nil {
			meta, payload, err := s.openEnvelope(raw)
			if err != nil {
				return err
			}
			clear(payload)
			key := recordKey{Address: meta.Address, ArtifactDigest: meta.ArtifactDigest, Attempt: meta.Attempt}
			if envelopePath != s.envelopePath(key) {
				return errors.New("rollback envelope path does not match record identity")
			}
			return removeLegacyArtifacts(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func removeLegacyArtifacts(dir string) error {
	for _, name := range []string{"metadata.json", "payload.bin"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}
