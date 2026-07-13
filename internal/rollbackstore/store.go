// Package rollbackstore persists encrypted, bounded rollback payloads below
// the agent state directory.
package rollbackstore

import (
	"context"
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
	"sync"
	"time"
)

var ErrCapacity = errors.New("rollback storage capacity exhausted")

// KeyProvider supplies the endpoint-local encryption key. Deployments may
// prefer a TPM-backed implementation; RootKeyProvider is the root-only fallback.
type KeyProvider interface {
	LoadOrCreate(context.Context, string) ([]byte, error)
}

// RootKeyProvider persists an AES-256 key in a root-only file.
type RootKeyProvider struct{}

func (RootKeyProvider) LoadOrCreate(_ context.Context, root string) ([]byte, error) {
	path := filepath.Join(root, "rollback.key")
	if key, err := os.ReadFile(path); err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("rollback key has invalid length")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := writeAtomic(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

type Options struct {
	Root        string
	MaxBytes    int64
	MaxAttempts int
	MaxAge      time.Duration
	Now         func() time.Time
	KeyProvider KeyProvider
}

type Record struct {
	Address        string
	ArtifactDigest string
	Attempt        int
	Payload        []byte
	Armed          bool
	Sensitive      bool
	Successful     bool
}

type Store struct {
	root        string
	maxBytes    int64
	maxAttempts int
	maxAge      time.Duration
	now         func() time.Time
	key         []byte
	mu          sync.Mutex
}

type metadata struct {
	Address        string    `json:"address"`
	ArtifactDigest string    `json:"artifact_digest"`
	Attempt        int       `json:"attempt"`
	CreatedAt      time.Time `json:"created_at"`
	Armed          bool      `json:"armed"`
	Sensitive      bool      `json:"sensitive"`
	Successful     bool      `json:"successful"`
	Nonce          []byte    `json:"nonce"`
	Checksum       string    `json:"checksum"`
}

func New(opts Options) (*Store, error) {
	if opts.Root == "" {
		return nil, errors.New("rollback root is required")
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 64 << 20
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 10
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 30 * 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.KeyProvider == nil {
		opts.KeyProvider = RootKeyProvider{}
	}
	if err := os.MkdirAll(filepath.Join(opts.Root, "records"), 0o700); err != nil {
		return nil, err
	}
	key, err := opts.KeyProvider.LoadOrCreate(context.Background(), opts.Root)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("rollback key provider did not return AES-256 key")
	}
	return &Store{root: opts.Root, maxBytes: opts.MaxBytes, maxAttempts: opts.MaxAttempts, maxAge: opts.MaxAge, now: opts.Now, key: key}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Save(_ context.Context, record Record) error {
	if record.Address == "" || record.ArtifactDigest == "" || record.Attempt <= 0 {
		return errors.New("rollback record requires address, artifact digest, and positive attempt")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, record.Payload, nil)
	meta := metadata{Address: record.Address, ArtifactDigest: record.ArtifactDigest, Attempt: record.Attempt, CreatedAt: s.now().UTC(), Armed: record.Armed, Sensitive: record.Sensitive, Successful: record.Successful, Nonce: nonce}
	sum := sha256.Sum256(record.Payload)
	meta.Checksum = hex.EncodeToString(sum[:])
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := s.cleanupLocked(); err != nil {
		return err
	}
	used, err := directorySize(filepath.Join(s.root, "records"))
	if err != nil {
		return err
	}
	if used+int64(len(ciphertext)+len(encoded)) > s.maxBytes {
		return ErrCapacity
	}
	dir := s.recordDir(record.Address, record.ArtifactDigest, record.Attempt)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, "payload.bin"), ciphertext, 0o600); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "metadata.json"), encoded, 0o600)
}

func (s *Store) Load(_ context.Context, address, digest string, attempt int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.recordDir(address, digest, attempt)
	encoded, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var meta metadata
	if err := json.Unmarshal(encoded, &meta); err != nil {
		return nil, err
	}
	ciphertext, err := os.ReadFile(filepath.Join(dir, "payload.bin"))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	payload, err := gcm.Open(nil, meta.Nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt rollback payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != meta.Checksum {
		return nil, errors.New("rollback payload checksum mismatch")
	}
	return payload, nil
}

// Delete removes one rollback payload after acknowledgement or terminal
// recovery. The caller retains any non-secret transaction audit metadata.
func (s *Store) Delete(_ context.Context, address, digest string, attempt int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(s.recordDir(address, digest, attempt))
}

func (s *Store) cleanupLocked() error {
	root := filepath.Join(s.root, "records")
	cutoff := s.now().Add(-s.maxAge)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "metadata.json" {
			return err
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var meta metadata
		if err := json.Unmarshal(encoded, &meta); err != nil {
			return err
		}
		if !meta.Armed && meta.CreatedAt.Before(cutoff) {
			return os.RemoveAll(filepath.Dir(path))
		}
		return nil
	})
}

func (s *Store) recordDir(address, digest string, attempt int) string {
	addressSum := sha256.Sum256([]byte(address))
	digestSum := sha256.Sum256([]byte(digest))
	return filepath.Join(s.root, "records", hex.EncodeToString(addressSum[:]), hex.EncodeToString(digestSum[:]), fmt.Sprintf("%d", attempt))
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
