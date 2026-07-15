package secrets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRemotrProviderResolvesThroughScopedEndpointAPI(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/secrets/resolve" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var request ResolveRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Reference != "remotr:repositories/private" || request.ArtifactDigest != "sha256:artifact" || request.ResourceAddress != "base/private" || request.Purpose != "repository-credential" {
			t.Fatalf("request = %#v", request)
		}
		body, _ := json.Marshal(Resolved{Provider: ProviderRemotr, Version: "7", Fingerprint: "sha256:safe", Material: []byte("machine token value")})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}

	provider := NewRemotrProvider("https://server.example.test", client)
	resolved, err := provider.Resolve(context.Background(), ResolveRequest{
		Reference: "remotr:repositories/private", ArtifactDigest: "sha256:artifact",
		ResourceAddress: "base/private", Purpose: "repository-credential",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Material) != "machine token value" || resolved.Version != "7" || resolved.Fingerprint != "sha256:safe" {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestRoutingResolverNeverFallsBackAcrossProviders(t *testing.T) {
	remotr := resolverFunc(func(context.Context, ResolveRequest) (Resolved, error) {
		return Resolved{Provider: ProviderRemotr, Material: []byte("remotr")}, nil
	})
	resolver := NewRoutingResolver(nil, remotr)
	if _, err := resolver.Resolve(context.Background(), ResolveRequest{Reference: "local-file:/root/missing", ResourceAddress: "base/db", Purpose: "password"}); err == nil {
		t.Fatal("missing local-file provider fell back to Remotr")
	}
}

type resolverFunc func(context.Context, ResolveRequest) (Resolved, error)

func (f resolverFunc) Resolve(ctx context.Context, request ResolveRequest) (Resolved, error) {
	return f(ctx, request)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
