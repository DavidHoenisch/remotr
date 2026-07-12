// Package polling defines deterministic Sync polling delays.
package polling

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"time"
)

const (
	defaultInterval = 30 * time.Second
	maxJitter       = 3 * time.Second
	retryBase       = time.Second
	retryMax        = 5 * time.Minute
	permanentDelay  = 15 * time.Minute
)

// Clock is the timer boundary used by a polling loop.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

// Random supplies the bounded randomness used for startup spreading.
type Random interface {
	Int64N(n int64) int64
}

// SystemRandom supplies process-local random startup spreading.
func SystemRandom() Random { return systemRandom{} }

// Policy bounds both initial and recurring Sync delays.
type Policy struct {
	Interval       time.Duration
	StartupMax     time.Duration
	JitterMax      time.Duration
	RetryBase      time.Duration
	RetryMax       time.Duration
	PermanentDelay time.Duration
}

// NewPolicy returns a bounded policy for the requested Sync interval. Startup
// delay and stable jitter are each at most one tenth of the interval, capped at
// three seconds. Therefore a successful Sync is followed by another attempt
// within 1.1 times the configured interval.
func NewPolicy(interval time.Duration) Policy {
	if interval <= 0 {
		interval = defaultInterval
	}
	spread := interval / 10
	if spread > maxJitter {
		spread = maxJitter
	}
	return Policy{
		Interval:       interval,
		StartupMax:     spread,
		JitterMax:      spread,
		RetryBase:      retryBase,
		RetryMax:       retryMax,
		PermanentDelay: permanentDelay,
	}
}

// Backoff tracks transient Sync failures until a successful cycle resets it.
type Backoff struct {
	policy   Policy
	random   Random
	attempts int
}

// NewBackoff creates a bounded jittered retry policy.
func NewBackoff(policy Policy, random Random) *Backoff {
	return &Backoff{policy: policy, random: random}
}

// NextDelay returns a capped exponential delay with bounded positive jitter.
func (b *Backoff) NextDelay() time.Duration {
	base, cap := b.policy.RetryBase, b.policy.RetryMax
	if base <= 0 {
		base = retryBase
	}
	if cap < base {
		cap = base
	}
	delay := base
	for i := 0; i < b.attempts && delay < cap; i++ {
		if delay > cap/2 {
			delay = cap
			break
		}
		delay *= 2
	}
	b.attempts++
	if delay >= cap || b.random == nil {
		return delay
	}
	span := delay
	if remaining := cap - delay; span > remaining {
		span = remaining
	}
	if span <= 0 {
		return delay
	}
	return delay + time.Duration(b.random.Int64N(span.Nanoseconds()+1))
}

// Reset clears failure history after a successful Sync.
func (b *Backoff) Reset() { b.attempts = 0 }

// RetryAfterDelay clamps a server-requested retry delay to the transient cap.
func (p Policy) RetryAfterDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	cap := p.RetryMax
	if cap <= 0 {
		cap = retryMax
	}
	if delay > cap {
		return cap
	}
	return delay
}

// StartupDelay returns a bounded randomized delay before the first Sync.
func (p Policy) StartupDelay(random Random) time.Duration {
	if random == nil || p.StartupMax <= 0 {
		return 0
	}
	return time.Duration(random.Int64N(p.StartupMax.Nanoseconds()))
}

// SuccessDelay uses a stable endpoint-derived jitter so endpoints with the
// same interval stay spread after successful Sync attempts.
func (p Policy) SuccessDelay(endpointID string) time.Duration {
	if p.Interval <= 0 {
		return 0
	}
	if p.JitterMax <= 0 {
		return p.Interval
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(endpointID))
	jitter := time.Duration(h.Sum64() % uint64(p.JitterMax.Nanoseconds()+1))
	return p.Interval + jitter
}

// Wait blocks through the supplied clock until delay elapses or context ends.
func Wait(ctx context.Context, clock Clock, delay time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if delay <= 0 {
		return true
	}
	if clock == nil {
		clock = realClock{}
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(delay):
		return true
	}
}

type realClock struct{}

func (realClock) After(delay time.Duration) <-chan time.Time { return time.After(delay) }

type systemRandom struct{}

func (systemRandom) Int64N(n int64) int64 { return rand.Int64N(n) }
