package secrets

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

type memorySecret struct {
	nextVersion uint64
	generation  uint64
	active      string
	versions    map[string]StoredVersion
}

type MemoryVersionRepository struct {
	mu      sync.RWMutex
	secrets map[string]*memorySecret
	now     func() time.Time
}

func NewMemoryVersionRepository() *MemoryVersionRepository {
	return &MemoryVersionRepository{secrets: make(map[string]*memorySecret), now: func() time.Time { return time.Now().UTC() }}
}

func (m *MemoryVersionRepository) secret(name string) *memorySecret {
	secret := m.secrets[name]
	if secret == nil {
		secret = &memorySecret{nextVersion: 1, versions: make(map[string]StoredVersion)}
		m.secrets[name] = secret
	}
	return secret
}

func (m *MemoryVersionRepository) AllocateVersion(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret := m.secret(name)
	version := strconv.FormatUint(secret.nextVersion, 10)
	secret.nextVersion++
	return version, nil
}

func (m *MemoryVersionRepository) CreateVersion(_ context.Context, stored StoredVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret := m.secret(stored.Record.Scope.Name)
	version := stored.Record.Scope.Version
	if _, exists := secret.versions[version]; exists {
		return fmt.Errorf("secret version already exists")
	}
	secret.versions[version] = cloneStoredVersion(stored)
	return nil
}

func (m *MemoryVersionRepository) GetExactVersion(_ context.Context, name, version string) (StoredVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	secret := m.secrets[name]
	if secret == nil {
		return StoredVersion{}, ErrVersionNotFound
	}
	stored, ok := secret.versions[version]
	if !ok {
		return StoredVersion{}, ErrVersionNotFound
	}
	stored.Active = secret.active == version
	if stored.Active {
		stored.ActivationGeneration = secret.generation
	}
	return cloneStoredVersion(stored), nil
}

func (m *MemoryVersionRepository) GetActiveVersion(ctx context.Context, name string) (StoredVersion, error) {
	m.mu.RLock()
	secret := m.secrets[name]
	active := ""
	if secret != nil {
		active = secret.active
	}
	m.mu.RUnlock()
	if active == "" {
		return StoredVersion{}, ErrVersionNotFound
	}
	return m.GetExactVersion(ctx, name, active)
}

func (m *MemoryVersionRepository) ListVersions(_ context.Context, name string) ([]StoredVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	secret := m.secrets[name]
	if secret == nil {
		return []StoredVersion{}, nil
	}
	out := make([]StoredVersion, 0, len(secret.versions))
	for version, stored := range secret.versions {
		stored.Active = secret.active == version
		if stored.Active {
			stored.ActivationGeneration = secret.generation
		}
		out = append(out, cloneStoredVersion(stored))
	}
	sort.Slice(out, func(i, j int) bool {
		left, _ := strconv.ParseUint(out[i].Record.Scope.Version, 10, 64)
		right, _ := strconv.ParseUint(out[j].Record.Scope.Version, 10, 64)
		return left < right
	})
	return out, nil
}

func (m *MemoryVersionRepository) ActivationGeneration(_ context.Context, name string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if secret := m.secrets[name]; secret != nil {
		return secret.generation, nil
	}
	return 0, ErrVersionNotFound
}

func (m *MemoryVersionRepository) ActivateVersion(_ context.Context, name, version string, generation uint64, actor string, rollouts []RolloutBinding) (StoredVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret := m.secrets[name]
	if secret == nil || generation != secret.generation+1 {
		return StoredVersion{}, fmt.Errorf("secret activation generation conflict")
	}
	stored, ok := secret.versions[version]
	if !ok {
		return StoredVersion{}, ErrVersionNotFound
	}
	if stored.RevokedAt != nil {
		return StoredVersion{}, ErrVersionRevoked
	}
	now := m.now()
	stored.ActivatedAt = &now
	stored.ActivatedBy = actor
	stored.Rollouts = append([]RolloutBinding(nil), rollouts...)
	secret.versions[version] = stored
	secret.active = version
	secret.generation = generation
	stored.Active = true
	stored.ActivationGeneration = generation
	return cloneStoredVersion(stored), nil
}

func (m *MemoryVersionRepository) RevokeVersion(_ context.Context, name, version, actor string) (StoredVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret := m.secrets[name]
	if secret == nil {
		return StoredVersion{}, ErrVersionNotFound
	}
	stored, ok := secret.versions[version]
	if !ok {
		return StoredVersion{}, ErrVersionNotFound
	}
	if stored.RevokedAt == nil {
		now := m.now()
		stored.RevokedAt = &now
		stored.RevokedBy = actor
		secret.versions[version] = stored
	}
	stored.Active = secret.active == version
	if stored.Active {
		stored.ActivationGeneration = secret.generation
	}
	return cloneStoredVersion(stored), nil
}

func cloneStoredVersion(stored StoredVersion) StoredVersion {
	stored.Record = stored.Record.Clone()
	stored.Rollouts = append([]RolloutBinding(nil), stored.Rollouts...)
	if stored.ActivatedAt != nil {
		activated := *stored.ActivatedAt
		stored.ActivatedAt = &activated
	}
	if stored.RevokedAt != nil {
		revoked := *stored.RevokedAt
		stored.RevokedAt = &revoked
	}
	return stored
}
