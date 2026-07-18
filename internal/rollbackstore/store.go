// Package rollbackstore persists encrypted, bounded rollback payloads below
// the agent state directory.
package rollbackstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrCapacity        = errors.New("rollback storage capacity exhausted")
	ErrExpired         = errors.New("rollback payload expired")
	ErrArmedRecovery   = errors.New("armed rollback recovery blocks mutation")
	ErrRecoveryBlocked = errors.New("armed rollback recovery is unavailable")
)

const MaxSensitiveRetention = 24 * time.Hour

type Options struct {
	Root                string
	MaxBytes            int64
	MaxAttempts         int
	MaxAge              time.Duration
	FilesystemAllowance int64
	Now                 func() time.Time
	KeyProvider         KeyProvider
	AvailableBytes      func(string) (int64, error)
	CrashInjector       func(DurabilityPoint) error
}

type Record struct {
	Address        string
	ArtifactDigest string
	Attempt        int
	Payload        []byte
	Armed          bool
	Sensitive      bool
	Successful     bool
	ExpiresAt      time.Time
}

type Store struct {
	root                string
	maxBytes            int64
	maxAttempts         int
	maxAge              time.Duration
	now                 func() time.Time
	key                 []byte
	keyID               string
	protection          ProtectionReport
	keyProvider         KeyProvider
	historicalKeys      map[string][]byte
	armed               map[recordKey]struct{}
	reservations        map[recordKey]reservationEntry
	nextReservationID   uint64
	filesystemAllowance int64
	availableBytes      func(string) (int64, error)
	crashInjector       func(DurabilityPoint) error
	mu                  sync.Mutex
}

type metadata struct {
	Version        int       `json:"version,omitempty"`
	State          Lifecycle `json:"state,omitempty"`
	Address        string    `json:"address"`
	ArtifactDigest string    `json:"artifact_digest"`
	Attempt        int       `json:"attempt"`
	CreatedAt      time.Time `json:"created_at"`
	Armed          bool      `json:"armed"`
	Sensitive      bool      `json:"sensitive"`
	Successful     bool      `json:"successful"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	KeyID          string    `json:"key_id,omitempty"`
	Nonce          []byte    `json:"nonce"`
	Checksum       string    `json:"checksum"`
	PayloadPresent bool      `json:"payload_present,omitempty"`
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
	if opts.FilesystemAllowance <= 0 {
		opts.FilesystemAllowance = defaultFilesystemAllowance
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.KeyProvider == nil {
		opts.KeyProvider = defaultKeyProvider()
	}
	if opts.AvailableBytes == nil {
		opts.AvailableBytes = filesystemAvailableBytes
	}
	if err := os.MkdirAll(filepath.Join(opts.Root, "records"), 0o700); err != nil {
		return nil, err
	}
	material, err := opts.KeyProvider.LoadOrCreate(context.Background(), opts.Root)
	if err != nil {
		return nil, err
	}
	if len(material.Key) != 32 || !validKeyID(material.ID) || !material.Protection.valid() {
		clear(material.Key)
		return nil, errors.New("rollback key provider did not return AES-256 key")
	}
	protection := ProtectionReport{Class: material.Protection, KeyID: material.ID}
	if material.Protection == ProtectionRootFile {
		protection.ReducedProtection = true
		protection.Limitation = RootCompromiseLimitation
	}
	store := &Store{
		root: opts.Root, maxBytes: opts.MaxBytes, maxAttempts: opts.MaxAttempts,
		maxAge: opts.MaxAge, now: opts.Now, key: material.Key, keyID: material.ID, protection: protection,
		keyProvider: opts.KeyProvider, historicalKeys: make(map[string][]byte),
		armed:        make(map[recordKey]struct{}),
		reservations: make(map[recordKey]reservationEntry), filesystemAllowance: opts.FilesystemAllowance,
		availableBytes: opts.AvailableBytes, crashInjector: opts.CrashInjector,
	}
	if err := store.cleanupTemporaryEnvelopes(); err != nil {
		return nil, err
	}
	if err := store.migrateLegacyRecords(context.Background()); err != nil {
		return nil, err
	}
	if err := store.scanArmed(context.Background()); err != nil {
		return nil, err
	}
	store.mu.Lock()
	err = store.cleanupLocked()
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Root() string { return s.root }

// Protection returns a safe view of the selected rollback key protection.
func (s *Store) Protection() ProtectionReport { return s.protection }

// RotateKey changes the active rollback key identity after binding any legacy
// unversioned envelopes to the current identity. Existing keys remain
// decrypt-only through the provider until their referenced records are gone.
func (s *Store) RotateKey(ctx context.Context) (ProtectionReport, error) {
	if err := ctx.Err(); err != nil {
		return ProtectionReport{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rotating, ok := s.keyProvider.(RotatingKeyProvider)
	if !ok {
		return ProtectionReport{}, fmt.Errorf("%w: selected provider does not support key rotation", ErrKeyProtectionUnavailable)
	}
	if err := s.bindUnversionedEnvelopesLocked(); err != nil {
		return ProtectionReport{}, err
	}
	material, err := rotating.Rotate(ctx, s.root)
	if err != nil {
		return ProtectionReport{}, err
	}
	if len(material.Key) != 32 || !validKeyID(material.ID) || material.ID == s.keyID || material.Protection != s.protection.Class {
		clear(material.Key)
		return ProtectionReport{}, errors.New("rotated rollback key has invalid material, identity, or protection class")
	}
	s.historicalKeys[s.keyID] = s.key
	s.key = material.Key
	s.keyID = material.ID
	s.protection.KeyID = material.ID
	return s.protection, nil
}

func (s *Store) Save(_ context.Context, record Record) error {
	if record.Address == "" || record.ArtifactDigest == "" || record.Attempt <= 0 {
		return errors.New("rollback record requires address, artifact digest, and positive attempt")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(record, nil)
}

func (s *Store) saveLocked(record Record, reservation *reservationEntry) error {
	if err := s.ensureMutationAllowedLocked(record.Address); err != nil {
		return err
	}
	allowedReservationID := uint64(0)
	if reservation != nil {
		allowedReservationID = reservation.ID
	}
	if err := s.ensureReservationOwnerLocked(record.Address, allowedReservationID); err != nil {
		return err
	}
	now := s.now().UTC()
	if record.Sensitive && (!record.ExpiresAt.After(now) || record.ExpiresAt.After(now.Add(MaxSensitiveRetention))) {
		return fmt.Errorf("sensitive rollback expiry must be within %s", MaxSensitiveRetention)
	}

	state := LifecycleStaged
	if record.Armed {
		state = LifecycleArmed
	} else if record.Successful {
		state = LifecycleAcknowledged
	}
	meta := metadata{
		Version: RecordVersion, State: state,
		Address: record.Address, ArtifactDigest: record.ArtifactDigest, Attempt: record.Attempt,
		CreatedAt: now, Armed: record.Armed, Sensitive: record.Sensitive,
		Successful: record.Successful, ExpiresAt: record.ExpiresAt.UTC(),
	}
	encoded, err := s.sealEnvelope(meta, record.Payload, true)
	if err != nil {
		return err
	}
	if err := s.cleanupLocked(); err != nil {
		return err
	}
	required := int64(len(encoded)) + s.filesystemAllowance
	if reservation != nil && required > reservation.RequiredBytes {
		return ErrCapacity
	}
	key := recordKey{Address: record.Address, ArtifactDigest: record.ArtifactDigest, Attempt: record.Attempt}
	if err := s.pruneToConfiguredLimitLocked(required, key); err != nil {
		return err
	}
	used, err := s.configuredUsageLocked(key)
	if err != nil {
		return err
	}
	reserved := s.reservedBytesLocked(key)
	if used+reserved+required > s.maxBytes {
		return ErrCapacity
	}
	available, err := s.availableBytes(s.root)
	if err != nil {
		return err
	}
	if reserved+required > available {
		return ErrCapacity
	}
	if err := s.writeEnvelopeAtomic(s.recordDir(record.Address, record.ArtifactDigest, record.Attempt), encoded); err != nil {
		return err
	}
	if record.Armed {
		s.armed[key] = struct{}{}
	}
	return nil
}

func (s *Store) Load(_ context.Context, address, digest string, attempt int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := recordKey{Address: address, ArtifactDigest: digest, Attempt: attempt}
	meta, payload, err := s.readEnvelope(key)
	if err != nil {
		return nil, err
	}
	if meta.Sensitive && !meta.ExpiresAt.After(s.now().UTC()) {
		clear(payload)
		if err := s.transitionAndDropPayloadLocked(key, LifecycleExpired); err != nil {
			return nil, err
		}
		return nil, ErrExpired
	}
	if !meta.PayloadPresent {
		return nil, os.ErrNotExist
	}
	return payload, nil
}

// Delete acknowledges one rollback payload and retains bounded safe metadata.
func (s *Store) Delete(_ context.Context, address, digest string, attempt int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := recordKey{Address: address, ArtifactDigest: digest, Attempt: attempt}
	if _, err := os.Stat(s.envelopePath(key)); errors.Is(err, os.ErrNotExist) {
		delete(s.armed, key)
		return nil
	} else if err != nil {
		return err
	}
	return s.transitionAndDropPayloadLocked(key, LifecycleAcknowledged)
}

func (s *Store) cleanupLocked() error {
	return s.applyRetentionLocked()
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
