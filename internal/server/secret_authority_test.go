package server

import "testing"

// OS-LSM-082, OS-LSM-083: an authenticated Sync token is stable until
// authority changes, absent during a mutation, and different after the
// mutation completes.
func TestSecretAuthorityTokenTracksStableMutationBoundary(t *testing.T) {
	cache := newUnchangedSyncCache(FastPathConfig{
		Enabled: true,
		Backend: FastPathMemory,
	})
	first := cache.secretAuthorityToken("endpoint-1", "production")
	if first == "" {
		t.Fatal("stable authority token is empty")
	}
	if repeated := cache.secretAuthorityToken("endpoint-1", "production"); repeated != first {
		t.Fatalf("stable authority token changed: %q != %q", repeated, first)
	}

	complete := cache.beginMutation(cacheScopeGlobal, "")
	if unstable := cache.secretAuthorityToken("endpoint-1", "production"); unstable != "" {
		t.Fatalf("unstable authority token = %q, want empty", unstable)
	}
	complete()
	changed := cache.secretAuthorityToken("endpoint-1", "production")
	if changed == "" || changed == first {
		t.Fatalf("changed authority token = %q, prior %q", changed, first)
	}
}
