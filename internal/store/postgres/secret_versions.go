package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type SecretQuerier interface {
	AllocateSecretVersion(context.Context, string) (int64, error)
	CreateSecretVersion(context.Context, db.CreateSecretVersionParams) error
	GetExactSecretVersion(context.Context, db.GetExactSecretVersionParams) (db.SecretVersion, error)
	GetActiveSecretVersion(context.Context, string) (db.SecretVersion, error)
	ListSecretVersions(context.Context, string) ([]db.SecretVersion, error)
	GetSecretActivationGeneration(context.Context, string) (int64, error)
	ActivateSecretVersion(context.Context, db.ActivateSecretVersionParams) (db.SecretVersion, error)
	RevokeSecretVersion(context.Context, db.RevokeSecretVersionParams) (db.SecretVersion, error)
}

var (
	_ SecretQuerier             = (*db.Queries)(nil)
	_ secrets.VersionRepository = (*Store)(nil)
)

func (s *Store) AllocateVersion(ctx context.Context, name string) (string, error) {
	if s.secretQ == nil {
		return "", fmt.Errorf("secret version queries are unavailable")
	}
	version, err := s.secretQ.AllocateSecretVersion(ctx, name)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(version, 10), nil
}

func (s *Store) CreateVersion(ctx context.Context, stored secrets.StoredVersion) error {
	if s.secretQ == nil {
		return fmt.Errorf("secret version queries are unavailable")
	}
	version, err := postgresSecretVersionNumber(stored.Record.Scope.Version)
	if err != nil {
		return err
	}
	envelopeJSON, err := json.Marshal(stored.Record)
	if err != nil {
		return fmt.Errorf("encode secret envelope: %w", err)
	}
	return s.secretQ.CreateSecretVersion(ctx, db.CreateSecretVersionParams{
		Name: stored.Record.Scope.Name, Version: version, EnvelopeJson: envelopeJSON,
		CreatedAt: pgtype.Timestamptz{Time: stored.CreatedAt.UTC(), Valid: true}, CreatedBy: stored.CreatedBy,
	})
}

func (s *Store) GetExactVersion(ctx context.Context, name, version string) (secrets.StoredVersion, error) {
	if s.secretQ == nil {
		return secrets.StoredVersion{}, fmt.Errorf("secret version queries are unavailable")
	}
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return secrets.StoredVersion{}, err
	}
	row, err := s.secretQ.GetExactSecretVersion(ctx, db.GetExactSecretVersionParams{Name: name, Version: number})
	return storedSecretVersion(row, err)
}

func (s *Store) GetActiveVersion(ctx context.Context, name string) (secrets.StoredVersion, error) {
	if s.secretQ == nil {
		return secrets.StoredVersion{}, fmt.Errorf("secret version queries are unavailable")
	}
	row, err := s.secretQ.GetActiveSecretVersion(ctx, name)
	return storedSecretVersion(row, err)
}

func (s *Store) ListVersions(ctx context.Context, name string) ([]secrets.StoredVersion, error) {
	if s.secretQ == nil {
		return nil, fmt.Errorf("secret version queries are unavailable")
	}
	rows, err := s.secretQ.ListSecretVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make([]secrets.StoredVersion, 0, len(rows))
	for _, row := range rows {
		stored, err := storedSecretVersion(row, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, stored)
	}
	return out, nil
}

func (s *Store) ActivationGeneration(ctx context.Context, name string) (uint64, error) {
	if s.secretQ == nil {
		return 0, fmt.Errorf("secret version queries are unavailable")
	}
	generation, err := s.secretQ.GetSecretActivationGeneration(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, secrets.ErrVersionNotFound
	}
	if err != nil {
		return 0, err
	}
	if generation < 0 {
		return 0, fmt.Errorf("invalid secret activation generation")
	}
	return uint64(generation), nil
}

func (s *Store) ActivateVersion(ctx context.Context, name, version string, generation uint64, actor string, rollouts []secrets.RolloutBinding) (secrets.StoredVersion, error) {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return secrets.StoredVersion{}, err
	}
	if generation > uint64(^uint64(0)>>1) {
		return secrets.StoredVersion{}, fmt.Errorf("secret activation generation exceeds Postgres bigint")
	}
	rolloutsJSON, err := json.Marshal(rollouts)
	if err != nil {
		return secrets.StoredVersion{}, err
	}
	row, err := s.secretQ.ActivateSecretVersion(ctx, db.ActivateSecretVersionParams{
		Name: name, Version: number, ActivationGeneration: int64(generation), ActivatedBy: actor, RolloutsJson: rolloutsJSON,
	})
	return storedSecretVersion(row, err)
}

func (s *Store) RevokeVersion(ctx context.Context, name, version, actor string) (secrets.StoredVersion, error) {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return secrets.StoredVersion{}, err
	}
	row, err := s.secretQ.RevokeSecretVersion(ctx, db.RevokeSecretVersionParams{Name: name, Version: number, RevokedBy: actor})
	return storedSecretVersion(row, err)
}

func storedSecretVersion(row db.SecretVersion, queryErr error) (secrets.StoredVersion, error) {
	if errors.Is(queryErr, pgx.ErrNoRows) {
		return secrets.StoredVersion{}, secrets.ErrVersionNotFound
	}
	if queryErr != nil {
		return secrets.StoredVersion{}, queryErr
	}
	var record secrets.EncryptedRecord
	if err := json.Unmarshal(row.EnvelopeJson, &record); err != nil {
		return secrets.StoredVersion{}, fmt.Errorf("decode stored secret envelope: %w", err)
	}
	var rollouts []secrets.RolloutBinding
	if len(row.RolloutsJson) > 0 {
		if err := json.Unmarshal(row.RolloutsJson, &rollouts); err != nil {
			return secrets.StoredVersion{}, fmt.Errorf("decode stored secret rollouts: %w", err)
		}
	}
	if row.ActivationGeneration < 0 {
		return secrets.StoredVersion{}, fmt.Errorf("invalid stored activation generation")
	}
	return secrets.StoredVersion{
		Record: record, CreatedAt: pgTime(row.CreatedAt), CreatedBy: row.CreatedBy, Active: row.Active,
		ActivationGeneration: uint64(row.ActivationGeneration), ActivatedAt: optionalPGTime(row.ActivatedAt), ActivatedBy: row.ActivatedBy,
		RevokedAt: optionalPGTime(row.RevokedAt), RevokedBy: row.RevokedBy, Rollouts: rollouts,
	}, nil
}

func postgresSecretVersionNumber(value string) (int64, error) {
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version <= 0 || strconv.FormatInt(version, 10) != value {
		return 0, fmt.Errorf("invalid secret version")
	}
	return version, nil
}

func pgTime(value pgtype.Timestamptz) time.Time { return value.Time.UTC() }

func optionalPGTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}
