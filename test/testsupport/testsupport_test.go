package testsupport

import (
	"testing"
	"time"
)

func TestClockAndRandomAreDeterministic(t *testing.T) {
	clock := NewClock(time.Unix(1, 0))
	clock.Advance(time.Second)
	if got := clock.Now(); !got.Equal(time.Unix(2, 0)) {
		t.Fatalf("now = %s", got)
	}
	if Random(4).Uint64() != Random(4).Uint64() {
		t.Fatal("random source is not reproducible")
	}
}

func TestSecretCanaryIsStableAndRecognizable(t *testing.T) {
	if got := SecretCanary("one"); got != SecretCanary("one") || got[:19] != "remotr-test-secret-" {
		t.Fatalf("canary = %q", got)
	}
}

func TestContextAndCleanup(t *testing.T) {
	ctx := Context(t, time.Second)
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	called := false
	t.Cleanup(func() {
		if !called {
			t.Fatal("cleanup was not called")
		}
	})
	Cleanup(t, func() error { called = true; return nil })
}
