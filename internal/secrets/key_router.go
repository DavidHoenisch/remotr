package secrets

import (
	"context"
	"fmt"
	"strings"
)

// KeyEncryptionRouter encrypts new DEKs with one active provider while
// retaining historical providers for unwrap and cross-provider rotation.
type KeyEncryptionRouter struct {
	active    KeyEncryptionProvider
	providers map[string]KeyEncryptionProvider
}

func NewKeyEncryptionRouter(active KeyEncryptionProvider, historical ...KeyEncryptionProvider) (*KeyEncryptionRouter, error) {
	if active == nil {
		return nil, fmt.Errorf("active key-encryption provider is required")
	}
	router := &KeyEncryptionRouter{active: active, providers: make(map[string]KeyEncryptionProvider, len(historical)+1)}
	for _, provider := range append([]KeyEncryptionProvider{active}, historical...) {
		if provider == nil {
			return nil, fmt.Errorf("key-encryption provider is required")
		}
		id := provider.ProviderID()
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("key-encryption provider identifier is invalid")
		}
		if _, exists := router.providers[id]; exists {
			return nil, fmt.Errorf("duplicate key-encryption provider %q", id)
		}
		router.providers[id] = provider
	}
	return router, nil
}

func (r *KeyEncryptionRouter) ProviderID() string { return r.active.ProviderID() }

func (r *KeyEncryptionRouter) ActiveKeyID(ctx context.Context) (string, error) {
	return r.active.ActiveKeyID(ctx)
}

func (r *KeyEncryptionRouter) WrapDEK(ctx context.Context, dek, aad []byte) (WrappedKey, error) {
	return r.active.WrapDEK(ctx, dek, aad)
}

func (r *KeyEncryptionRouter) UnwrapDEK(ctx context.Context, wrapped WrappedKey, aad []byte) ([]byte, error) {
	provider, ok := r.providers[wrapped.ProviderID]
	if !ok {
		return nil, fmt.Errorf("key-encryption provider %q is unavailable", wrapped.ProviderID)
	}
	return provider.UnwrapDEK(ctx, wrapped, aad)
}

func (r *KeyEncryptionRouter) KeyAvailable(ctx context.Context, providerID, keyID string) (bool, error) {
	provider, ok := r.providers[providerID]
	if !ok {
		return false, nil
	}
	return provider.KeyAvailable(ctx, providerID, keyID)
}

var _ KeyEncryptionProvider = (*KeyEncryptionRouter)(nil)
