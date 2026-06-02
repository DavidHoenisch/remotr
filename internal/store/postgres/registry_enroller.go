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

func (r *RegistryEnroller) ConsumeEnrollmentToken(token string) (string, bool) {
	tok, err := r.Store.ConsumeEnrollmentToken(context.Background(), token)
	if err != nil {
		return "", false
	}
	return tok.Fleet, true
}

func (r *RegistryEnroller) RegisterEndpoint(e registry.Endpoint) error {
	_, err := r.Store.RegisterEndpoint(context.Background(), e.ID, e.Fleet, e.CertFingerprint)
	return err
}
