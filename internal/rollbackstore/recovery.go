package rollbackstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// RecordVersion is the lifecycle metadata version written by the current
// rollback store. Version zero identifies the pre-lifecycle split format.
const RecordVersion = 1

// Lifecycle is the durable state of one rollback transaction.
type Lifecycle string

const (
	LifecycleReserved     Lifecycle = "reserved"
	LifecycleStaged       Lifecycle = "staged"
	LifecycleArmed        Lifecycle = "armed"
	LifecycleAcknowledged Lifecycle = "acknowledged"
	LifecycleRolledBack   Lifecycle = "rolled_back"
	LifecycleExpired      Lifecycle = "expired"
	LifecycleSuperseded   Lifecycle = "superseded"
	LifecycleAbandoned    Lifecycle = "abandoned"
)

func (state Lifecycle) valid() bool {
	switch state {
	case LifecycleReserved, LifecycleStaged, LifecycleArmed, LifecycleAcknowledged,
		LifecycleRolledBack, LifecycleExpired, LifecycleSuperseded, LifecycleAbandoned:
		return true
	default:
		return false
	}
}

// Recovery is one validated armed transaction delivered to the startup
// recovery seam. Payload is plaintext only for the duration of the callback.
type Recovery struct {
	Address        string
	ArtifactDigest string
	Attempt        int
	Payload        []byte
}

type recordKey struct {
	Address        string
	ArtifactDigest string
	Attempt        int
}

// RecoverArmed restores every validated armed transaction discovered at
// startup. A successful callback advances the record to rolled_back and
// destroys its payload; a failed callback leaves it armed and blocking.
func (s *Store) RecoverArmed(ctx context.Context, recover func(context.Context, Recovery) error) error {
	if recover == nil {
		return errors.New("rollback recovery callback is required")
	}
	keys := s.armedKeys()
	for _, key := range keys {
		payload, err := s.Load(ctx, key.Address, key.ArtifactDigest, key.Attempt)
		if err != nil {
			return fmt.Errorf("%w for resource %q", ErrRecoveryBlocked, key.Address)
		}
		recovery := Recovery{
			Address: key.Address, ArtifactDigest: key.ArtifactDigest,
			Attempt: key.Attempt, Payload: payload,
		}
		if err := recover(ctx, recovery); err != nil {
			clear(payload)
			return fmt.Errorf("%w for resource %q", ErrRecoveryBlocked, key.Address)
		}
		clear(payload)
		if err := s.markRolledBack(key); err != nil {
			return fmt.Errorf("%w for resource %q", ErrRecoveryBlocked, key.Address)
		}
	}
	return nil
}

func (s *Store) scanArmed(ctx context.Context) error {
	root := filepath.Join(s.root, "records")
	var keys []recordKey
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || info.Name() != "metadata.json" {
			return walkErr
		}
		raw, err := os.ReadFile(path)
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
		if filepath.Clean(path) != filepath.Join(s.recordDir(key.Address, key.ArtifactDigest, key.Attempt), "metadata.json") {
			return errors.New("rollback metadata path does not match record identity")
		}
		if meta.State != LifecycleArmed {
			return nil
		}
		payload, err := s.Load(ctx, key.Address, key.ArtifactDigest, key.Attempt)
		if err != nil {
			return err
		}
		clear(payload)
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w", ErrRecoveryBlocked)
	}
	for _, key := range keys {
		s.armed[key] = struct{}{}
	}
	return nil
}

func normalizeMetadata(meta *metadata) error {
	if meta.Address == "" || meta.ArtifactDigest == "" || meta.Attempt <= 0 {
		return errors.New("rollback metadata has an invalid record identity")
	}
	if meta.Version == 0 {
		switch {
		case meta.Armed:
			meta.State = LifecycleArmed
		case meta.Successful:
			meta.State = LifecycleAcknowledged
		default:
			meta.State = LifecycleStaged
		}
		return nil
	}
	if meta.Version != RecordVersion || !meta.State.valid() {
		return errors.New("rollback metadata has an unsupported lifecycle version or state")
	}
	if (meta.State == LifecycleArmed) != meta.Armed {
		return errors.New("rollback metadata armed flag does not match lifecycle")
	}
	return nil
}

func (s *Store) ensureMutationAllowedLocked(address string) error {
	for key := range s.armed {
		if key.Address == address {
			return fmt.Errorf("%w for resource %q", ErrArmedRecovery, address)
		}
	}
	return nil
}

func (s *Store) armedKeys() []recordKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]recordKey, 0, len(s.armed))
	for key := range s.armed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Address != keys[j].Address {
			return keys[i].Address < keys[j].Address
		}
		if keys[i].ArtifactDigest != keys[j].ArtifactDigest {
			return keys[i].ArtifactDigest < keys[j].ArtifactDigest
		}
		return keys[i].Attempt < keys[j].Attempt
	})
	return keys
}

func (s *Store) markRolledBack(key recordKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.recordDir(key.Address, key.ArtifactDigest, key.Attempt)
	path := filepath.Join(dir, "metadata.json")
	raw, err := os.ReadFile(path)
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
	if meta.State != LifecycleArmed {
		return errors.New("rollback transaction is no longer armed")
	}
	meta.Version = RecordVersion
	meta.State = LifecycleRolledBack
	meta.Armed = false
	meta.Nonce = nil
	meta.Checksum = ""
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := writeAtomic(path, encoded, 0o600); err != nil {
		return err
	}
	delete(s.armed, key)
	if err := os.Remove(filepath.Join(dir, "payload.bin")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
