package executor

import (
	"context"
	"strings"
)

type packageMetadataRefreshKey struct{}

type packageMetadataRefreshBoundaries map[string]func(context.Context) error

// WithPackageMetadataRefresh attaches one run-scoped native metadata refresh
// boundary. Providers fall back to their own process boundary when no engine
// coordination is present.
func WithPackageMetadataRefresh(ctx context.Context, provider string, refresh func(context.Context) error) context.Context {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || refresh == nil {
		return ctx
	}
	existing, _ := ctx.Value(packageMetadataRefreshKey{}).(packageMetadataRefreshBoundaries)
	boundaries := make(packageMetadataRefreshBoundaries, len(existing)+1)
	for name, boundary := range existing {
		boundaries[name] = boundary
	}
	boundaries[provider] = refresh
	return context.WithValue(ctx, packageMetadataRefreshKey{}, boundaries)
}

// PackageMetadataRefresh returns the run-scoped refresh boundary for provider.
func PackageMetadataRefresh(ctx context.Context, provider string) (func(context.Context) error, bool) {
	boundaries, ok := ctx.Value(packageMetadataRefreshKey{}).(packageMetadataRefreshBoundaries)
	refresh := boundaries[strings.ToLower(strings.TrimSpace(provider))]
	return refresh, ok && refresh != nil
}
