package rollbackstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const defaultFilesystemAllowance int64 = 4 << 10

// JSON time values omit trailing fractional zeros, so the encoded metadata can
// grow between Reserve and Arm even though the schema is unchanged. Keep a
// small explicit bound for those variable-width fields and future compatible
// metadata additions.
const reservationMetadataAllowance int64 = 256

// ReservationRequest describes the complete rollback payload that must fit
// before a provider is allowed to mutate its resource.
type ReservationRequest struct {
	Address        string
	ArtifactDigest string
	Attempt        int
	PayloadBytes   int64
	Sensitive      bool
	ExpiresAt      time.Time
}

// Reservation is a single-use capacity claim. Arm persists the protected
// payload before provider mutation; Release abandons an unused claim.
type Reservation struct {
	store   *Store
	key     recordKey
	id      uint64
	request ReservationRequest
}

type reservationEntry struct {
	ID            uint64
	RequiredBytes int64
}

// Reserve claims configured and filesystem capacity for one complete
// encrypted rollback record without writing the payload.
func (s *Store) Reserve(_ context.Context, request ReservationRequest) (*Reservation, error) {
	if request.Address == "" || request.ArtifactDigest == "" || request.Attempt <= 0 || request.PayloadBytes < 0 {
		return nil, errors.New("rollback reservation requires address, artifact digest, positive attempt, and non-negative payload size")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureMutationAllowedLocked(request.Address); err != nil {
		return nil, err
	}
	for key := range s.reservations {
		if key.Address == request.Address {
			return nil, errors.New("rollback reservation already exists for resource")
		}
	}
	if err := s.validateSensitiveExpiry(request.Sensitive, request.ExpiresAt); err != nil {
		return nil, err
	}
	if err := s.cleanupLocked(); err != nil {
		return nil, err
	}
	required, err := s.estimatedFootprint(request)
	if err != nil {
		return nil, err
	}
	key := recordKey{Address: request.Address, ArtifactDigest: request.ArtifactDigest, Attempt: request.Attempt}
	if err := s.pruneToConfiguredLimitLocked(required, key); err != nil {
		return nil, err
	}
	used, err := s.configuredUsageLocked(key)
	if err != nil {
		return nil, err
	}
	reserved := s.reservedBytesLocked(recordKey{})
	if used+reserved+required > s.maxBytes {
		return nil, ErrCapacity
	}
	available, err := s.availableBytes(s.root)
	if err != nil {
		return nil, err
	}
	if reserved+required > available {
		return nil, ErrCapacity
	}
	s.nextReservationID++
	entry := reservationEntry{ID: s.nextReservationID, RequiredBytes: required}
	s.reservations[key] = entry
	return &Reservation{store: s, key: key, id: entry.ID, request: request}, nil
}

// Arm consumes the reservation and durably protects payload. Providers may
// mutate only after Arm succeeds.
func (r *Reservation) Arm(_ context.Context, payload []byte) error {
	if r == nil || r.store == nil {
		return errors.New("rollback reservation is required")
	}
	s := r.store
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.reservations[r.key]
	if !ok || entry.ID != r.id {
		return errors.New("rollback reservation is no longer active")
	}
	defer delete(s.reservations, r.key)
	if int64(len(payload)) > r.request.PayloadBytes {
		return ErrCapacity
	}
	return s.saveLocked(Record{
		Address: r.request.Address, ArtifactDigest: r.request.ArtifactDigest,
		Attempt: r.request.Attempt, Payload: payload, Armed: true,
		Sensitive: r.request.Sensitive, ExpiresAt: r.request.ExpiresAt,
	}, &entry)
}

// Release abandons an unused capacity reservation.
func (r *Reservation) Release() {
	if r == nil || r.store == nil {
		return
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if entry, ok := r.store.reservations[r.key]; ok && entry.ID == r.id {
		delete(r.store.reservations, r.key)
	}
}

func (s *Store) estimatedFootprint(request ReservationRequest) (int64, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}
	meta := metadata{
		Version: RecordVersion, State: LifecycleArmed,
		Address: request.Address, ArtifactDigest: request.ArtifactDigest, Attempt: request.Attempt,
		CreatedAt: s.now().UTC(), Armed: true, Sensitive: request.Sensitive,
		ExpiresAt: request.ExpiresAt.UTC(), KeyID: s.keyID, Nonce: make([]byte, gcm.NonceSize()),
		PayloadPresent: true,
	}
	header, err := json.Marshal(envelopeHeader{Version: envelopeVersion, Metadata: meta})
	if err != nil {
		return 0, err
	}
	ciphertextBytes := request.PayloadBytes + int64(gcm.Overhead())
	if ciphertextBytes < 0 || ciphertextBytes > math.MaxInt64-2 {
		return 0, ErrCapacity
	}
	base64Bytes := ((ciphertextBytes + 2) / 3) * 4
	const envelopeFraming = int64(len(`{"header":`)) + int64(len(`,"ciphertext":"`)) + int64(len(`"}`))
	if base64Bytes > math.MaxInt64-int64(len(header))-envelopeFraming-s.filesystemAllowance-reservationMetadataAllowance {
		return 0, ErrCapacity
	}
	return envelopeFraming + int64(len(header)) + base64Bytes + s.filesystemAllowance + reservationMetadataAllowance, nil
}

func (s *Store) validateSensitiveExpiry(sensitive bool, expiresAt time.Time) error {
	if !sensitive {
		return nil
	}
	now := s.now().UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(MaxSensitiveRetention)) {
		return errors.New("sensitive rollback expiry is outside the permitted retention window")
	}
	return nil
}

// configuredUsageLocked returns final configured-cap usage. Replacing one
// unarmed record does not count both its old and new final footprints.
func (s *Store) configuredUsageLocked(replacing recordKey) (int64, error) {
	root := filepath.Join(s.root, "records")
	used, err := directorySize(root)
	if err != nil {
		return 0, err
	}
	count := int64(0)
	err = filepath.Walk(root, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && info.Name() == envelopeFilename {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	used += count * s.filesystemAllowance
	if replacing.Address == "" {
		return used, nil
	}
	dir := s.recordDir(replacing.Address, replacing.ArtifactDigest, replacing.Attempt)
	current, err := directorySize(dir)
	if err == nil {
		return used - current - s.filesystemAllowance, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return used, nil
	}
	return 0, err
}

// reservedBytesLocked excludes one reservation when it is being consumed.
func (s *Store) reservedBytesLocked(exclude recordKey) int64 {
	var total int64
	for key, entry := range s.reservations {
		if key == exclude {
			continue
		}
		total += entry.RequiredBytes
	}
	return total
}

func (s *Store) ensureReservationOwnerLocked(address string, allowedID uint64) error {
	for key, entry := range s.reservations {
		if key.Address == address && entry.ID != allowedID {
			return errors.New("rollback reservation already exists for resource")
		}
	}
	return nil
}

func filesystemAvailableBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	blockSize := uint64(stat.Bsize)
	if blockSize == 0 {
		return 0, errors.New("rollback filesystem reported a zero block size")
	}
	if uint64(stat.Bavail) > uint64(math.MaxInt64)/blockSize {
		return math.MaxInt64, nil
	}
	return int64(uint64(stat.Bavail) * blockSize), nil
}
