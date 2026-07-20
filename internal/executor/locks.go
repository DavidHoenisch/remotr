package executor

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// LockManager serializes operations that share one or more lock domains.
type LockManager struct {
	mu    sync.Mutex
	locks map[string]chan struct{}
}

// LockCoordinator acquires process-scoped operation lock domains. Engines
// accept this boundary so package coordination can be verified without
// relying on scheduler timing or native package-manager locks.
type LockCoordinator interface {
	Acquire(context.Context, []string) (func(), error)
}

// NewLockManager constructs an empty process-local lock manager.
func NewLockManager() *LockManager {
	return &LockManager{locks: make(map[string]chan struct{})}
}

// Acquire waits for every requested domain until the caller context expires.
// Domains are deduplicated and sorted to prevent lock-order deadlocks.
func (m *LockManager) Acquire(ctx context.Context, domains []string) (func(), error) {
	domains = NormalizeLockDomains(domains)
	acquired := make([]chan struct{}, 0, len(domains))
	for _, domain := range domains {
		lock := m.lock(domain)
		select {
		case <-ctx.Done():
			releaseLocks(acquired)
			return nil, ctx.Err()
		case <-lock:
			acquired = append(acquired, lock)
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() { releaseLocks(acquired) })
	}, nil
}

func (m *LockManager) lock(domain string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lock, ok := m.locks[domain]; ok {
		return lock
	}
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	m.locks[domain] = lock
	return lock
}

// NormalizeLockDomains trims, deduplicates, and sorts operation lock domains.
func NormalizeLockDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

func releaseLocks(locks []chan struct{}) {
	for i := len(locks) - 1; i >= 0; i-- {
		locks[i] <- struct{}{}
	}
}

// NativeLocker acquires provider-native locks with the same bounded context
// used for process-local lock domains.
type NativeLocker interface {
	AcquireNativeLocks(context.Context) (func(), error)
}
