package postgres

import (
	"context"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// RegistryEnroller adapts Store to registry.Enroller for HTTP enrollment handlers.
type RegistryEnroller struct {
	*Store
}

var _ registry.Enroller = (*RegistryEnroller)(nil)

func (r *RegistryEnroller) RedeemEnrollmentToken(token string) (string, bool) {
	ctx := context.Background()
	tok, err := r.Store.ConsumeEnrollmentToken(ctx, token)
	if err == nil {
		return tok.Fleet, true
	}
	return r.Store.RedeemDeploymentToken(ctx, token)
}

func (r *RegistryEnroller) RegisterEndpoint(e registry.Endpoint) error {
	_, err := r.Store.RegisterEndpoint(context.Background(), e.ID, e.Fleet, e.CertFingerprint)
	return err
}
