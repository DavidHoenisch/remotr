package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/test/testsupport"
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
		if request.Reference != "remotr:repositories/private@active" || request.ArtifactDigest != "sha256:artifact" || request.ResourceAddress != "base/private" || request.Purpose != "repository-credential" {
			t.Fatalf("request = %#v", request)
		}
		body, _ := json.Marshal(Resolved{Provider: ProviderRemotr, Version: "7", Fingerprint: "sha256:safe", Material: []byte("machine token value")})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}

	provider := NewRemotrProvider("https://server.example.test", client)
	resolved, err := provider.Resolve(context.Background(), ResolveRequest{
		Reference: "remotr:repositories/private@active", ArtifactDigest: "sha256:artifact",
		ResourceAddress: "base/private", Purpose: "repository-credential",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Material) != "machine token value" || resolved.Version != "7" || resolved.Fingerprint != "sha256:safe" {
		t.Fatalf("resolved = %#v", resolved)
	}
}

// OS-LSM-021/064/074: the real endpoint HTTP boundary must preserve the
// authorization classification that lets the agent report safe activation
// bootstrap evidence without retaining an untrusted response body.
func TestRemotrProviderClassifiesAuthorizationStatusesWithoutRetainingBody(t *testing.T) {
	canary := testsupport.SecretCanary("remotr-provider-authorization")
	for _, test := range []struct {
		name             string
		status           int
		wantUnauthorized bool
	}{
		{name: "unauthenticated", status: http.StatusUnauthorized, wantUnauthorized: true},
		{name: "forbidden", status: http.StatusForbidden, wantUnauthorized: true},
		{name: "provider unavailable", status: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Body:       io.NopCloser(strings.NewReader(canary)),
					Header:     make(http.Header),
				}, nil
			})}
			provider := NewRemotrProvider("https://server.example.test", client)
			_, err := provider.Resolve(t.Context(), ResolveRequest{
				Reference: "remotr:ubuntu-pro/prod-engineering@active", ArtifactDigest: "sha256:artifact",
				ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token",
			})
			if err == nil {
				t.Fatal("Resolve() error = nil")
			}
			if got := errors.Is(err, ErrUnauthorized); got != test.wantUnauthorized {
				t.Fatalf("errors.Is(ErrUnauthorized) = %v, want %v: %v", got, test.wantUnauthorized, err)
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatal("Resolve() retained the provider response body")
			}
		})
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
