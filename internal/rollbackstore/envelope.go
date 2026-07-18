package rollbackstore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	envelopeVersion       = 1
	envelopeFilename      = "transaction.envelope"
	temporaryEnvelopeHead = ".transaction-envelope-"
	temporaryEnvelopeTail = ".tmp"
)

// DurabilityPoint identifies an injectable boundary in atomic envelope
// activation. It exists for deterministic crash-recovery evidence.
type DurabilityPoint string

const (
	AfterTemporaryCreate  DurabilityPoint = "after-temporary-create"
	AfterEnvelopeWrite    DurabilityPoint = "after-envelope-write"
	AfterEnvelopeSync     DurabilityPoint = "after-envelope-sync"
	AfterEnvelopeActivate DurabilityPoint = "after-envelope-activate"
	AfterDirectorySync    DurabilityPoint = "after-directory-sync"
)

type envelopeHeader struct {
	Version  int      `json:"version"`
	Metadata metadata `json:"metadata"`
}

type transactionEnvelope struct {
	Header     envelopeHeader `json:"header"`
	Ciphertext []byte         `json:"ciphertext"`
}

func (s *Store) sealEnvelope(meta metadata, payload []byte, present bool) ([]byte, error) {
	meta.Version = RecordVersion
	meta.PayloadPresent = present
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	meta.Nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, meta.Nonce); err != nil {
		return nil, err
	}
	if present {
		sum := sha256.Sum256(payload)
		meta.Checksum = hex.EncodeToString(sum[:])
	} else {
		meta.Checksum = ""
		payload = nil
	}
	header := envelopeHeader{Version: envelopeVersion, Metadata: meta}
	aad, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, meta.Nonce, payload, aad)
	return json.Marshal(transactionEnvelope{Header: header, Ciphertext: ciphertext})
}

func (s *Store) openEnvelope(raw []byte) (metadata, []byte, error) {
	envelope, err := decodeEnvelope(raw)
	if err != nil {
		return metadata{}, nil, err
	}
	meta := envelope.Header.Metadata
	if envelope.Header.Version != envelopeVersion {
		return metadata{}, nil, errors.New("rollback envelope version is unsupported")
	}
	if err := normalizeMetadata(&meta); err != nil {
		return metadata{}, nil, err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return metadata{}, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return metadata{}, nil, err
	}
	if len(meta.Nonce) != gcm.NonceSize() || len(envelope.Ciphertext) < gcm.Overhead() {
		return metadata{}, nil, errors.New("rollback envelope protected payload is malformed")
	}
	aad, err := json.Marshal(envelope.Header)
	if err != nil {
		return metadata{}, nil, err
	}
	payload, err := gcm.Open(nil, meta.Nonce, envelope.Ciphertext, aad)
	if err != nil {
		return metadata{}, nil, errors.New("rollback envelope authentication failed")
	}
	if !meta.PayloadPresent {
		if len(payload) != 0 || meta.Checksum != "" {
			clear(payload)
			return metadata{}, nil, errors.New("payload-free rollback envelope contains protected payload fields")
		}
		return meta, nil, nil
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != meta.Checksum {
		clear(payload)
		return metadata{}, nil, errors.New("rollback envelope checksum mismatch")
	}
	return meta, payload, nil
}

func decodeEnvelope(raw []byte) (transactionEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope transactionEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return transactionEnvelope{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return transactionEnvelope{}, errors.New("rollback envelope has a trailing JSON value")
		}
		return transactionEnvelope{}, err
	}
	return envelope, nil
}

func (s *Store) writeEnvelopeAtomic(dir string, raw []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, temporaryEnvelopeHead+"*"+temporaryEnvelopeTail)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := s.injectCrash(AfterTemporaryCreate); err != nil {
		_ = tmp.Close()
		return err
	}
	n, err := tmp.Write(raw)
	if err != nil || n != len(raw) {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		if err == nil {
			err = io.ErrShortWrite
		}
		return err
	}
	if err := s.injectCrash(AfterEnvelopeWrite); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := s.injectCrash(AfterEnvelopeSync); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	finalPath := filepath.Join(dir, envelopeFilename)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := s.injectCrash(AfterEnvelopeActivate); err != nil {
		return err
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
	if closeErr != nil {
		return closeErr
	}
	return s.injectCrash(AfterDirectorySync)
}

func (s *Store) injectCrash(point DurabilityPoint) error {
	if s.crashInjector == nil {
		return nil
	}
	return s.crashInjector(point)
}

func (s *Store) cleanupTemporaryEnvelopes() error {
	root := filepath.Join(s.root, "records")
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		if strings.HasPrefix(info.Name(), temporaryEnvelopeHead) && strings.HasSuffix(info.Name(), temporaryEnvelopeTail) {
			return os.Remove(path)
		}
		return nil
	})
}

func (s *Store) envelopePath(key recordKey) string {
	return filepath.Join(s.recordDir(key.Address, key.ArtifactDigest, key.Attempt), envelopeFilename)
}

func (s *Store) readEnvelope(key recordKey) (metadata, []byte, error) {
	raw, err := os.ReadFile(s.envelopePath(key))
	if err != nil {
		return metadata{}, nil, err
	}
	meta, payload, err := s.openEnvelope(raw)
	if err != nil {
		return metadata{}, nil, err
	}
	if meta.Address != key.Address || meta.ArtifactDigest != key.ArtifactDigest || meta.Attempt != key.Attempt {
		clear(payload)
		return metadata{}, nil, fmt.Errorf("rollback envelope identity does not match its storage key")
	}
	return meta, payload, nil
}
