// Package testsupport provides deterministic helpers at external test seams.
package testsupport

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

// Clock is an externally controlled clock for deterministic timeout and retry tests.
type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewClock(now time.Time) *Clock { return &Clock{now: now} }
func (c *Clock) Now() time.Time     { c.mu.RLock(); defer c.mu.RUnlock(); return c.now }
func (c *Clock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

// Random returns a reproducible source for jitter and selection tests.
func Random(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

// SecretCanary returns a recognizable synthetic value that must never cross a
// redaction boundary. It is not a credential and must not be used in production.
func SecretCanary(label string) string {
	sum := sha256.Sum256([]byte(label))
	return fmt.Sprintf("remotr-test-secret-%x", sum[:8])
}

// Context creates a bounded context and guarantees cancellation at test cleanup.
func Context(t testing.TB, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// Cleanup registers a resource cleanup function and reports cleanup failures.
func Cleanup(t testing.TB, close func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := close(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
}
