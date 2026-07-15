package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RemotrProvider resolves server-managed values through the authenticated
// endpoint API. The supplied HTTP client owns the endpoint mTLS transport.
type RemotrProvider struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewRemotrProvider(baseURL string, client *http.Client) *RemotrProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &RemotrProvider{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: client}
}

func (p *RemotrProvider) Resolve(ctx context.Context, request ResolveRequest) (Resolved, error) {
	provider, _, err := ParseReference(request.Reference)
	if err != nil {
		return Resolved{}, err
	}
	if provider != ProviderRemotr {
		return Resolved{}, fmt.Errorf("Remotr provider cannot resolve %q", provider)
	}
	if err := ValidateRequest(request); err != nil {
		return Resolved{}, err
	}
	if strings.TrimSpace(request.ArtifactDigest) == "" {
		return Resolved{}, fmt.Errorf("artifact digest is required")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Resolved{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/secrets/resolve", bytes.NewReader(body))
	if err != nil {
		return Resolved{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := p.HTTPClient.Do(httpRequest)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve Remotr secret: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return Resolved{}, fmt.Errorf("resolve Remotr secret: status %d", response.StatusCode)
	}
	var resolved Resolved
	decoder := json.NewDecoder(io.LimitReader(response.Body, MaxMaterialBytes*2))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resolved); err != nil {
		return Resolved{}, fmt.Errorf("decode Remotr secret response: %w", err)
	}
	if resolved.Provider != ProviderRemotr || len(resolved.Material) == 0 || len(resolved.Material) > MaxMaterialBytes {
		return Resolved{}, fmt.Errorf("invalid Remotr secret response")
	}
	return resolved, nil
}

// RoutingResolver dispatches by the explicitly selected provider. It never
// falls back across providers because that would change secret semantics.
type RoutingResolver struct {
	local  Resolver
	remotr Resolver
}

func NewRoutingResolver(local, remotr Resolver) *RoutingResolver {
	return &RoutingResolver{local: local, remotr: remotr}
}

func (r *RoutingResolver) Resolve(ctx context.Context, request ResolveRequest) (Resolved, error) {
	provider, _, err := ParseReference(request.Reference)
	if err != nil {
		return Resolved{}, err
	}
	var resolver Resolver
	switch provider {
	case ProviderLocalFile:
		resolver = r.local
	case ProviderRemotr:
		resolver = r.remotr
	}
	if resolver == nil {
		return Resolved{}, fmt.Errorf("secret provider %q is unavailable", provider)
	}
	return resolver.Resolve(ctx, request)
}
