package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// OS-LSM-080, OS-PSA-019. Public seam: the endpoint uses the real Remotr
// provider at the external HTTPS boundary and suppresses the second request
// only after an authenticated Sync authority token has been observed.
func TestAuthorityCachingResolverMakesOneRequestForStableScope(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(Resolved{
			Provider: ProviderRemotr,
			Version:  "1",
			Material: []byte("secret-canary"),
		})
	}))
	t.Cleanup(server.Close)

	resolver := NewAuthorityCachingResolver(
		NewRemotrProvider(server.URL, server.Client()),
		AuthorityCacheOptions{MaxEntries: 8, MaxMaterialBytes: 1 << 20},
	)
	resolver.SetAuthorityToken("stable-token")
	request := ResolveRequest{
		Reference:       "remotr:repositories/private@active",
		ArtifactDigest:  "sha256:artifact",
		ResourceAddress: "base/apt_repository/private",
		Purpose:         "repository-credential",
	}
	first, err := resolver.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	first.Material[0] = 'X'
	second, err := resolver.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("secret resolution requests = %d, want 1", requests)
	}
	if string(second.Material) != "secret-canary" {
		t.Fatalf("cached material was aliased: %q", second.Material)
	}
}

// OS-LSM-081: local-file authority is independent of the server Sync token and
// therefore bypasses this cache even if the wrapper is called directly.
func TestAuthorityCachingResolverDoesNotCacheLocalFileProvider(t *testing.T) {
	delegate := &authorityTestResolver{resolved: Resolved{
		Provider: ProviderLocalFile, Material: []byte("local-canary"),
	}}
	resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
	resolver.SetAuthorityToken("server-token")
	request := ResolveRequest{
		Reference:       "local-file:/run/remotr/secret",
		ArtifactDigest:  "sha256:artifact",
		ResourceAddress: "base/resource",
		Purpose:         "test",
	}
	for range 2 {
		if _, err := resolver.Resolve(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if delegate.calls != 2 {
		t.Fatalf("local-file delegate calls = %d, want 2", delegate.calls)
	}
}

type authorityChangeResolver struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (r *authorityChangeResolver) Resolve(
	ctx context.Context,
	_ ResolveRequest,
) (Resolved, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		select {
		case <-r.release:
		case <-ctx.Done():
			return Resolved{}, ctx.Err()
		}
	}
	return Resolved{
		Provider: ProviderRemotr,
		Version:  fmt.Sprint(call),
		Material: []byte(fmt.Sprintf("canary-%d", call)),
	}, nil
}

func (r *authorityChangeResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// OS-LSM-082: a token change racing a remote miss discards that superseded
// result and retries under the current authority before returning to Apply.
func TestAuthorityCachingResolverRetriesConcurrentAuthorityChange(t *testing.T) {
	delegate := &authorityChangeResolver{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
	resolver.SetAuthorityToken("first")
	type outcome struct {
		resolved Resolved
		err      error
	}
	result := make(chan outcome, 1)
	go func() {
		resolved, err := resolver.Resolve(t.Context(), ResolveRequest{
			Reference:       "remotr:token@active",
			ArtifactDigest:  "sha256:artifact",
			ResourceAddress: "base/resource",
			Purpose:         "test",
		})
		result <- outcome{resolved: resolved, err: err}
	}()
	<-delegate.started
	resolver.SetAuthorityToken("second")
	close(delegate.release)
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.resolved.Version != "2" || string(got.resolved.Material) != "canary-2" {
		t.Fatalf("Resolve() = %+v, want current-authority result", got.resolved)
	}
	if calls := delegate.callCount(); calls != 2 {
		t.Fatalf("delegate calls = %d, want 2", calls)
	}
}

type authorityTestResolver struct {
	calls    int
	resolved Resolved
	err      error
}

func (r *authorityTestResolver) Resolve(
	_ context.Context,
	_ ResolveRequest,
) (Resolved, error) {
	r.calls++
	return cloneResolved(r.resolved), r.err
}

// OS-LSM-082, OS-LSM-083, OS-LSM-087, OS-LSM-088, OS-LSM-089: changed and
// missing authority fail closed, stable denials are local, and transient errors
// are retried.
func TestAuthorityCachingResolverInvalidationAndFailureClasses(t *testing.T) {
	request := ResolveRequest{
		Reference:       "remotr:repositories/private@active",
		ArtifactDigest:  "sha256:artifact",
		ResourceAddress: "base/apt_repository/private",
		Purpose:         "repository-credential",
	}

	t.Run("changed or missing token", func(t *testing.T) {
		delegate := &authorityTestResolver{resolved: Resolved{
			Provider: ProviderRemotr, Version: "1", Material: []byte("canary"),
		}}
		resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
		resolver.SetAuthorityToken("first")
		if _, err := resolver.Resolve(t.Context(), request); err != nil {
			t.Fatal(err)
		}
		resolver.SetAuthorityToken("second")
		if _, err := resolver.Resolve(t.Context(), request); err != nil {
			t.Fatal(err)
		}
		resolver.SetAuthorityToken("")
		for range 2 {
			if _, err := resolver.Resolve(t.Context(), request); err != nil {
				t.Fatal(err)
			}
		}
		if delegate.calls != 4 {
			t.Fatalf("delegate calls = %d, want 4", delegate.calls)
		}
	})

	t.Run("authorization denial", func(t *testing.T) {
		delegate := &authorityTestResolver{err: ErrUnauthorized}
		resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
		resolver.SetAuthorityToken("stable")
		for range 2 {
			if _, err := resolver.Resolve(t.Context(), request); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("Resolve() error = %v, want unauthorized", err)
			}
		}
		if delegate.calls != 1 {
			t.Fatalf("denial delegate calls = %d, want 1", delegate.calls)
		}
		resolver.SetAuthorityToken("changed")
		_, _ = resolver.Resolve(t.Context(), request)
		if delegate.calls != 2 {
			t.Fatalf("changed denial delegate calls = %d, want 2", delegate.calls)
		}
	})

	t.Run("transient failure", func(t *testing.T) {
		delegate := &authorityTestResolver{err: errors.New("temporary outage")}
		resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
		resolver.SetAuthorityToken("stable")
		for range 2 {
			_, _ = resolver.Resolve(t.Context(), request)
		}
		if delegate.calls != 2 {
			t.Fatalf("transient delegate calls = %d, want 2", delegate.calls)
		}
	})
}

// OS-LSM-085, OS-LSM-086, OS-PSA-021, OS-PSA-022: scope churn stays within both
// configured bounds and invalidation clears the cache-owned plaintext bytes.
func TestAuthorityCachingResolverBoundsAndClearsMaterial(t *testing.T) {
	delegate := &authorityTestResolver{resolved: Resolved{
		Provider: ProviderRemotr, Version: "1", Material: []byte("12345678"),
	}}
	resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{
		MaxEntries: 2, MaxMaterialBytes: 16,
	})
	resolver.SetAuthorityToken("stable")
	for _, digest := range []string{"one", "two", "three"} {
		_, err := resolver.Resolve(t.Context(), ResolveRequest{
			Reference:       "remotr:token@active",
			ArtifactDigest:  digest,
			ResourceAddress: "base/resource",
			Purpose:         "test",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, materialBytes := resolver.bounds()
	if entries != 2 || materialBytes != 16 {
		t.Fatalf("cache bounds = %d entries, %d bytes", entries, materialBytes)
	}
	var retained []byte
	resolver.mu.Lock()
	for _, entry := range resolver.entries {
		retained = entry.resolved.Material
		break
	}
	resolver.mu.Unlock()
	resolver.SetAuthorityToken("changed")
	if entries, materialBytes = resolver.bounds(); entries != 0 || materialBytes != 0 {
		t.Fatalf("invalidated bounds = %d entries, %d bytes", entries, materialBytes)
	}
	for _, value := range retained {
		if value != 0 {
			t.Fatalf("invalidated material was not cleared: %q", retained)
		}
	}
}

func BenchmarkAuthorityCachingResolverHit(b *testing.B) {
	delegate := &authorityTestResolver{resolved: Resolved{
		Provider: ProviderRemotr, Version: "1", Material: []byte("benchmark"),
	}}
	resolver := NewAuthorityCachingResolver(delegate, AuthorityCacheOptions{})
	resolver.SetAuthorityToken("stable")
	request := ResolveRequest{
		Reference:       "remotr:benchmark@active",
		ArtifactDigest:  "sha256:artifact",
		ResourceAddress: "base/benchmark",
		Purpose:         "benchmark",
	}
	if _, err := resolver.Resolve(b.Context(), request); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := resolver.Resolve(b.Context(), request); err != nil {
			b.Fatal(err)
		}
	}
}
