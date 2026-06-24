package postgres

import (
	"context"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

func (s *Store) Create(ctx context.Context, rec apppackages.PackageRecord) (apppackages.PackageRecord, error) {
	return s.CreateAppPackage(ctx, rec)
}

func (s *Store) Get(ctx context.Context, name, version string) (apppackages.PackageRecord, error) {
	return s.GetAppPackage(ctx, name, version)
}

func (s *Store) List(ctx context.Context, namePrefix string) ([]apppackages.PackageRecord, error) {
	return s.ListAppPackages(ctx, namePrefix)
}

func (s *Store) Delete(ctx context.Context, name, version string) error {
	return s.DeleteAppPackage(ctx, name, version)
}

var _ apppackages.Catalog = (*Store)(nil)
