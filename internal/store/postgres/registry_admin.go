package postgres

import (
	"context"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// RegistryAdmin adapts Store to registry.Admin for operator admin API handlers.
type RegistryAdmin struct {
	*Store
}

var _ registry.Admin = (*RegistryAdmin)(nil)

func (r *RegistryAdmin) HasOperators() bool {
	ok, err := r.Store.HasOperators(context.Background())
	if err != nil {
		return false
	}
	return ok
}

func (r *RegistryAdmin) RegisterOperatorCredential(fp string) error {
	return r.Store.RegisterOperatorCredential(context.Background(), fp)
}

func (r *RegistryAdmin) IsOperatorCredential(fp string) bool {
	return r.Store.IsOperatorCredential(context.Background(), fp)
}

func (r *RegistryAdmin) ListOperatorCredentials() ([]registry.OperatorCredential, error) {
	return r.Store.ListOperatorCredentials(context.Background())
}

func (r *RegistryAdmin) ListEndpoints() ([]registry.Endpoint, error) {
	return r.Store.ListEndpoints(context.Background())
}

func (r *RegistryAdmin) GetEndpoint(id string) (registry.Endpoint, bool, error) {
	return r.Store.GetEndpoint(context.Background(), id)
}

func (r *RegistryAdmin) CreateEnrollmentToken(token, fleet string, expiresAt time.Time) error {
	_, err := r.Store.CreateEnrollmentToken(context.Background(), token, fleet, expiresAt)
	return err
}
