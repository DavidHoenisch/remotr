package executor

import "context"

type packageMetadataRefreshKey struct{}

type packageMetadataRefreshBoundary struct {
	provider string
	refresh  func(context.Context) error
}

// WithPackageMetadataRefresh attaches one run-scoped native metadata refresh
// boundary. Providers fall back to their own process boundary when no engine
// coordination is present.
func WithPackageMetadataRefresh(ctx context.Context, provider string, refresh func(context.Context) error) context.Context {
	if refresh == nil {
		return ctx
	}
	return context.WithValue(ctx, packageMetadataRefreshKey{}, packageMetadataRefreshBoundary{provider: provider, refresh: refresh})
}

// PackageMetadataRefresh returns the run-scoped refresh boundary for provider.
func PackageMetadataRefresh(ctx context.Context, provider string) (func(context.Context) error, bool) {
	boundary, ok := ctx.Value(packageMetadataRefreshKey{}).(packageMetadataRefreshBoundary)
	return boundary.refresh, ok && boundary.provider == provider && boundary.refresh != nil
}
