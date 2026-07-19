package rollbackstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Handle binds one provider resource to the agent rollback store. It owns
// attempt allocation and exposes only lifecycle operations; providers never
// construct rollback paths or write recovery payloads beside managed state.
type Handle struct {
	store          *Store
	address        string
	artifactDigest string
	sensitive      bool

	mu      sync.Mutex
	attempt int
	owned   bool
}

// NewHandle creates a transaction handle for one resource and artifact.
func NewHandle(store *Store, address, artifactDigest string, sensitive bool) (*Handle, error) {
	if store == nil || address == "" || artifactDigest == "" {
		return nil, errors.New("rollback handle requires store, resource address, and artifact digest")
	}
	return &Handle{store: store, address: address, artifactDigest: artifactDigest, sensitive: sensitive}, nil
}

// Preflight proves that the store can reserve a recovery payload of the
// supplied size for this resource, then releases the non-enforcing probe.
// Arm repeats the reservation immediately before mutation and remains the
// authoritative safety boundary.
func (h *Handle) Preflight(ctx context.Context, payloadBytes int64) error {
	if h == nil || payloadBytes < 0 {
		return errors.New("rollback handle and non-negative payload size are required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owned {
		return fmt.Errorf("%w for resource %q", ErrArmedRecovery, h.address)
	}
	records, err := h.store.Records(ctx, h.address)
	if err != nil {
		return err
	}
	attempt := 1
	for _, record := range records {
		if record.Armed {
			return fmt.Errorf("%w for resource %q", ErrArmedRecovery, h.address)
		}
		if record.Attempt >= attempt {
			attempt = record.Attempt + 1
		}
	}
	expiresAt := time.Time{}
	if h.sensitive {
		expiresAt = h.store.now().UTC().Add(MaxSensitiveRetention)
	}
	reservation, err := h.store.Reserve(ctx, ReservationRequest{
		Address: h.address, ArtifactDigest: h.artifactDigest, Attempt: attempt,
		PayloadBytes: payloadBytes, Sensitive: h.sensitive, ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}
	reservation.Release()
	return nil
}

// Arm reserves and durably persists the complete recovery payload before the
// provider mutates its managed resource.
func (h *Handle) Arm(ctx context.Context, payload []byte) error {
	if h == nil {
		return errors.New("rollback handle is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owned {
		return fmt.Errorf("%w for resource %q", ErrArmedRecovery, h.address)
	}
	records, err := h.store.Records(ctx, h.address)
	if err != nil {
		return err
	}
	attempt := 1
	for _, record := range records {
		if record.Armed {
			return fmt.Errorf("%w for resource %q", ErrArmedRecovery, h.address)
		}
		if record.Attempt >= attempt {
			attempt = record.Attempt + 1
		}
	}
	expiresAt := time.Time{}
	if h.sensitive {
		expiresAt = h.store.now().UTC().Add(MaxSensitiveRetention)
	}
	reservation, err := h.store.Reserve(ctx, ReservationRequest{
		Address: h.address, ArtifactDigest: h.artifactDigest, Attempt: attempt,
		PayloadBytes: int64(len(payload)), Sensitive: h.sensitive, ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}
	if err := reservation.Arm(ctx, payload); err != nil {
		return err
	}
	h.attempt = attempt
	h.owned = true
	return nil
}

// Rollback restores the one armed recovery for this resource and destroys its
// plaintext payload after the callback succeeds. A reconstructed handle can
// therefore recover an interrupted provider attempt after process restart.
func (h *Handle) Rollback(ctx context.Context, restore func([]byte) error) error {
	if h == nil || restore == nil {
		return errors.New("rollback handle and restore callback are required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	record, err := h.armedRecord(ctx)
	if err != nil {
		return err
	}
	payload, err := h.store.Load(ctx, record.Address, record.ArtifactDigest, record.Attempt)
	if err != nil {
		return fmt.Errorf("%w: load resource recovery: %w", ErrRecoveryBlocked, err)
	}
	defer clear(payload)
	if err := restore(payload); err != nil {
		return fmt.Errorf("%w: restore resource recovery: %w", ErrRecoveryBlocked, err)
	}
	key := recordKey{Address: record.Address, ArtifactDigest: record.ArtifactDigest, Attempt: record.Attempt}
	if err := h.store.markRolledBack(key); err != nil {
		return fmt.Errorf("%w: complete resource recovery: %w", ErrRecoveryBlocked, err)
	}
	h.attempt = 0
	h.owned = false
	return nil
}

// Acknowledge completes a provider-owned transaction after a subsequent
// compliant Check proves the mutation converged.
func (h *Handle) Acknowledge(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.owned || h.attempt < 1 {
		return nil
	}
	if err := h.store.Delete(ctx, h.address, h.artifactDigest, h.attempt); err != nil {
		return err
	}
	h.attempt = 0
	h.owned = false
	return nil
}

// Owned reports whether this process armed the current transaction. A newly
// reconstructed handle returns false even when durable recovery is pending.
func (h *Handle) Owned() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.owned
}

func (h *Handle) armedRecord(ctx context.Context) (RecordInfo, error) {
	records, err := h.store.Records(ctx, h.address)
	if err != nil {
		return RecordInfo{}, err
	}
	var armed []RecordInfo
	for _, record := range records {
		if !record.Armed {
			continue
		}
		if h.owned && (record.ArtifactDigest != h.artifactDigest || record.Attempt != h.attempt) {
			continue
		}
		armed = append(armed, record)
	}
	if len(armed) == 0 {
		return RecordInfo{}, os.ErrNotExist
	}
	if len(armed) != 1 {
		return RecordInfo{}, fmt.Errorf("%w: resource %q has %d armed recoveries", ErrRecoveryBlocked, h.address, len(armed))
	}
	return armed[0], nil
}
