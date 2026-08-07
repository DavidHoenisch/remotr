// Package testsupport provides deterministic helpers at external test seams.
package testsupport

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
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

// RequireUbuntuGuestRelease asserts the guest is Ubuntu amd64 on one of the
// accepted LTS releases and returns that VERSION_ID for exact Facts wiring.
func RequireUbuntuGuestRelease(t testing.TB, accepted ...string) string {
	t.Helper()
	if len(accepted) == 0 {
		accepted = []string{"24.04", "26.04"}
	}
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") {
		t.Fatalf("ubuntu guest OS release = %q, %v", raw, err)
	}
	for _, release := range accepted {
		if strings.Contains(string(raw), `VERSION_ID="`+release+`"`) {
			return release
		}
	}
	t.Fatalf("ubuntu guest OS release = %q, want one of %v", raw, accepted)
	return ""
}
