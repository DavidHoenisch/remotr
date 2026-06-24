package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func (s *Store) CreateAppPackage(ctx context.Context, rec apppackages.PackageRecord) (apppackages.PackageRecord, error) {
	manifestJSON, err := apppackages.ManifestJSON(rec.Manifest)
	if err != nil {
		return apppackages.PackageRecord{}, err
	}
	id := rec.ID
	if id == "" {
		id = uuid.NewString()
	}
	row, err := s.q.CreateAppPackage(ctx, db.CreateAppPackageParams{
		ID:       pgUUID(id),
		Name:     rec.Name,
		Version:  rec.Version,
		S3Key:    rec.S3Key,
		Sha256:   rec.SHA256,
		Manifest: manifestJSON,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return apppackages.PackageRecord{}, apppackages.ErrAlreadyExists
		}
		return apppackages.PackageRecord{}, err
	}
	return appPackageFromRow(row)
}

func (s *Store) GetAppPackage(ctx context.Context, name, version string) (apppackages.PackageRecord, error) {
	row, err := s.q.GetAppPackage(ctx, db.GetAppPackageParams{Name: name, Version: version})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apppackages.PackageRecord{}, apppackages.ErrNotFound
		}
		return apppackages.PackageRecord{}, err
	}
	return appPackageFromRow(row)
}

func (s *Store) ListAppPackages(ctx context.Context, namePrefix string) ([]apppackages.PackageRecord, error) {
	rows, err := s.q.ListAppPackages(ctx, namePrefix)
	if err != nil {
		return nil, err
	}
	out := make([]apppackages.PackageRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := appPackageFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (s *Store) DeleteAppPackage(ctx context.Context, name, version string) error {
	n, err := s.q.DeleteAppPackage(ctx, db.DeleteAppPackageParams{Name: name, Version: version})
	if err != nil {
		return err
	}
	if n == 0 {
		return apppackages.ErrNotFound
	}
	return nil
}

func appPackageFromRow(row db.AppPackage) (apppackages.PackageRecord, error) {
	id, err := uuidString(row.ID)
	if err != nil {
		return apppackages.PackageRecord{}, err
	}
	manifest, err := apppackages.ManifestFromJSON(row.Manifest)
	if err != nil {
		return apppackages.PackageRecord{}, fmt.Errorf("manifest: %w", err)
	}
	return apppackages.PackageRecord{
		ID:        id,
		Name:      row.Name,
		Version:   row.Version,
		S3Key:     row.S3Key,
		SHA256:    row.Sha256,
		Manifest:  manifest,
		CreatedAt: row.CreatedAt.Time.UTC(),
	}, nil
}

func pgUUID(id string) pgtype.UUID {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}
