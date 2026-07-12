package polling

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestPolicyUsesBoundedStableJitter(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	first := policy.SuccessDelay("endpoint-a")
	second := policy.SuccessDelay("endpoint-a")
	if first != second {
		t.Fatalf("stable jitter changed: %s then %s", first, second)
	}
	if first < policy.Interval || first > policy.Interval+policy.JitterMax {
		t.Fatalf("success delay %s outside [%s, %s]", first, policy.Interval, policy.Interval+policy.JitterMax)
	}
}

func TestPolicySpreadsEndpointJitterWithinBounds(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	delays := map[time.Duration]bool{}
	for i := range 100 {
		delay := policy.SuccessDelay("endpoint-" + strconv.Itoa(i))
		if delay < policy.Interval || delay > policy.Interval+policy.JitterMax {
			t.Fatalf("delay %s outside bounds", delay)
		}
		delays[delay] = true
	}
	if len(delays) < 2 {
		t.Fatal("endpoint jitter did not spread distinct endpoint identities")
	}
}

func TestPolicyBoundsStartupDelayWithInjectedRandomness(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	if got := policy.StartupDelay(fixedRandom(0)); got != 0 {
		t.Fatalf("zero startup delay = %s, want 0", got)
	}
	if got := policy.StartupDelay(fixedRandom(policy.StartupMax.Nanoseconds() - 1)); got >= policy.StartupMax {
		t.Fatalf("startup delay %s must be below %s", got, policy.StartupMax)
	}
}

func TestWaitUsesInjectableClockAndContext(t *testing.T) {
	clock := newFakeClock()
	done := make(chan bool, 1)
	go func() { done <- Wait(context.Background(), clock, time.Second) }()
	clock.advance(time.Second)
	if ok := <-done; !ok {
		t.Fatal("Wait returned false after clock advanced")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Wait(ctx, clock, time.Second) {
		t.Fatal("Wait returned true for cancelled context")
	}
}

func TestBackoffCapsJittersAndResets(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	backoff := NewBackoff(policy, fixedRandom(0))

	if got := backoff.NextDelay(); got != policy.RetryBase {
		t.Fatalf("first retry = %s, want %s", got, policy.RetryBase)
	}
	if got := backoff.NextDelay(); got != 2*policy.RetryBase {
		t.Fatalf("second retry = %s, want %s", got, 2*policy.RetryBase)
	}
	for range 20 {
		if got := backoff.NextDelay(); got > policy.RetryMax {
			t.Fatalf("retry = %s, exceeds cap %s", got, policy.RetryMax)
		}
	}
	backoff.Reset()
	if got := backoff.NextDelay(); got != policy.RetryBase {
		t.Fatalf("retry after reset = %s, want %s", got, policy.RetryBase)
	}
}

func TestPermanentDelayIsDistinctAndBounded(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	if policy.PermanentDelay <= policy.RetryMax {
		t.Fatalf("permanent delay %s must be distinct from transient cap %s", policy.PermanentDelay, policy.RetryMax)
	}
}

func TestRetryAfterDelayIsBounded(t *testing.T) {
	policy := NewPolicy(30 * time.Second)
	if got := policy.RetryAfterDelay(0); got != 0 {
		t.Fatalf("zero retry-after = %s, want 0", got)
	}
	if got := policy.RetryAfterDelay(policy.RetryMax + time.Second); got != policy.RetryMax {
		t.Fatalf("retry-after = %s, want cap %s", got, policy.RetryMax)
	}
}

type fixedRandom int64

func (r fixedRandom) Int64N(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return int64(r) % n
}

type fakeClock struct{ waits chan chan time.Time }

func newFakeClock() *fakeClock { return &fakeClock{waits: make(chan chan time.Time, 1)} }

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.waits <- ch
	return ch
}

func (c *fakeClock) advance(elapsed time.Duration) {
	ch := <-c.waits
	ch <- time.Unix(0, 0).Add(elapsed)
}
