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
	AllocateSecretVersion(context.Context, db.AllocateSecretVersionParams) (int64, error)
	CreateSecretVersion(context.Context, db.CreateSecretVersionParams) error
	GetExactSecretVersion(context.Context, db.GetExactSecretVersionParams) (db.GetExactSecretVersionRow, error)
	GetActiveSecretVersion(context.Context, string) (db.GetActiveSecretVersionRow, error)
	ListSecretVersions(context.Context, string) ([]db.ListSecretVersionsRow, error)
	GetSecretActivationGeneration(context.Context, string) (int64, error)
	ActivateSecretVersion(context.Context, db.ActivateSecretVersionParams) (db.ActivateSecretVersionRow, error)
	RevokeSecretVersion(context.Context, db.RevokeSecretVersionParams) (db.RevokeSecretVersionRow, error)
	CreateSecretRollbackReference(context.Context, db.CreateSecretRollbackReferenceParams) error
	ListActiveSecretRollbackReferences(context.Context, db.ListActiveSecretRollbackReferencesParams) ([]db.SecretRollbackReference, error)
	AbandonSecretRollbackReferences(context.Context, db.AbandonSecretRollbackReferencesParams) error
	DeleteSecretVersion(context.Context, db.DeleteSecretVersionParams) (int64, error)
}

var (
	_ SecretQuerier             = (*db.Queries)(nil)
	_ secrets.VersionRepository = (*Store)(nil)
)

type secretBackupRestoreQuerier interface {
	ListSecretVersionEnvelopes(context.Context) ([][]byte, error)
}

type secretCollectionQuerier interface {
	ListLogicalSecrets(context.Context, db.ListLogicalSecretsParams) ([]db.ListLogicalSecretsRow, error)
}

// ListEncryptedSecretRecords loads every Postgres-backed encrypted version for
// fail-closed startup validation after a database and external keyring restore.
func (s *Store) ListEncryptedSecretRecords(ctx context.Context) ([]secrets.EncryptedRecord, error) {
	queries, ok := s.secretQ.(secretBackupRestoreQuerier)
	if !ok {
		return nil, errors.New("secret backup/restore queries are unavailable")
	}
	envelopes, err := queries.ListSecretVersionEnvelopes(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]secrets.EncryptedRecord, 0, len(envelopes))
	for _, encoded := range envelopes {
		var record secrets.EncryptedRecord
		if err := json.Unmarshal(encoded, &record); err != nil {
			return nil, errors.New("restored encrypted secret record is malformed")
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Store) AllocateVersion(ctx context.Context, name string, scope secrets.Scope, scopeID string) (string, error) {
	if s.secretQ == nil {
		return "", fmt.Errorf("secret version queries are unavailable")
	}
	version, err := s.secretQ.AllocateSecretVersion(ctx, db.AllocateSecretVersionParams{Name: name, ScopeType: string(scope), ScopeID: scopeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", secrets.ErrScopeImmutable
	}
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
	return storedSecretVersion(row.EnvelopeJson, row.CreatedAt, row.CreatedBy, row.Active, row.ActivationGeneration, row.ActivatedAt, row.ActivatedBy, row.RevokedAt, row.RevokedBy, row.RolloutsJson, err)
}

func (s *Store) GetActiveVersion(ctx context.Context, name string) (secrets.StoredVersion, error) {
	if s.secretQ == nil {
		return secrets.StoredVersion{}, fmt.Errorf("secret version queries are unavailable")
	}
	row, err := s.secretQ.GetActiveSecretVersion(ctx, name)
	return storedSecretVersion(row.EnvelopeJson, row.CreatedAt, row.CreatedBy, row.Active, row.ActivationGeneration, row.ActivatedAt, row.ActivatedBy, row.RevokedAt, row.RevokedBy, row.RolloutsJson, err)
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
		stored, err := storedSecretVersion(row.EnvelopeJson, row.CreatedAt, row.CreatedBy, row.Active, row.ActivationGeneration, row.ActivatedAt, row.ActivatedBy, row.RevokedAt, row.RevokedBy, row.RolloutsJson, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, stored)
	}
	return out, nil
}

func (s *Store) ListLogicalSecrets(ctx context.Context, cursor string, limit int) (secrets.LogicalSecretPage, error) {
	queries, ok := s.secretQ.(secretCollectionQuerier)
	if !ok {
		return secrets.LogicalSecretPage{}, fmt.Errorf("secret collection queries are unavailable")
	}
	rows, err := queries.ListLogicalSecrets(ctx, db.ListLogicalSecretsParams{Cursor: cursor, PageSize: int32(limit + 1)})
	if err != nil {
		return secrets.LogicalSecretPage{}, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	page := secrets.LogicalSecretPage{Items: make([]secrets.LogicalSecretSummary, 0, len(rows))}
	for _, row := range rows {
		if row.VersionCount < 1 || row.VersionCount > 1<<31 {
			return secrets.LogicalSecretPage{}, fmt.Errorf("stored secret version count is invalid")
		}
		summary := secrets.LogicalSecretSummary{
			Name: row.Name, Scope: secrets.Scope(row.ScopeType), VersionCount: int(row.VersionCount),
			Fingerprint: row.Fingerprint, CreatedAt: pgTime(row.CreatedAt), UpdatedAt: pgTime(row.UpdatedAt),
		}
		switch summary.Scope {
		case secrets.ScopeGlobal:
			if row.ScopeID != "" {
				return secrets.LogicalSecretPage{}, fmt.Errorf("stored global secret scope identifier is invalid")
			}
		case secrets.ScopeFleet:
			summary.Fleet = row.ScopeID
		case secrets.ScopeEndpoint:
			summary.EndpointID = row.ScopeID
		default:
			return secrets.LogicalSecretPage{}, fmt.Errorf("stored secret scope is invalid")
		}
		if row.ActiveVersion > 0 {
			summary.ActiveVersion = strconv.FormatInt(row.ActiveVersion, 10)
		}
		page.Items = append(page.Items, summary)
	}
	if more && len(page.Items) > 0 {
		page.NextCursor = page.Items[len(page.Items)-1].Name
	}
	return page, nil
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
		Name: name, Version: number, Column3: int64(generation), ActivatedBy: actor, RolloutsJson: rolloutsJSON,
	})
	return storedSecretVersion(row.EnvelopeJson, row.CreatedAt, row.CreatedBy, row.Active, row.ActivationGeneration, row.ActivatedAt, row.ActivatedBy, row.RevokedAt, row.RevokedBy, row.RolloutsJson, err)
}

func (s *Store) RevokeVersion(ctx context.Context, name, version, actor string) (secrets.StoredVersion, error) {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return secrets.StoredVersion{}, err
	}
	row, err := s.secretQ.RevokeSecretVersion(ctx, db.RevokeSecretVersionParams{Name: name, Version: number, RevokedBy: actor})
	return storedSecretVersion(row.EnvelopeJson, row.CreatedAt, row.CreatedBy, row.Active, row.ActivationGeneration, row.ActivatedAt, row.ActivatedBy, row.RevokedAt, row.RevokedBy, row.RolloutsJson, err)
}

func (s *Store) CreateRollbackReference(ctx context.Context, reference secrets.StoredRollbackReference) error {
	version, err := postgresSecretVersionNumber(reference.Version)
	if err != nil {
		return err
	}
	if reference.Attempt <= 0 {
		return fmt.Errorf("rollback attempt must be positive")
	}
	return s.secretQ.CreateSecretRollbackReference(ctx, db.CreateSecretRollbackReferenceParams{
		ID: reference.ID, Name: reference.Name, Version: version, Fingerprint: reference.Fingerprint,
		ResourceAddress: reference.ResourceAddress, ArtifactDigest: reference.ArtifactDigest, Attempt: int64(reference.Attempt),
		CreatedAt: pgtype.Timestamptz{Time: reference.CreatedAt.UTC(), Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: reference.ExpiresAt.UTC(), Valid: true}, Status: string(reference.Status),
	})
}

func (s *Store) ListActiveRollbackReferences(ctx context.Context, name, version string, now time.Time) ([]secrets.StoredRollbackReference, error) {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return nil, err
	}
	rows, err := s.secretQ.ListActiveSecretRollbackReferences(ctx, db.ListActiveSecretRollbackReferencesParams{
		Name: name, Version: number, Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]secrets.StoredRollbackReference, 0, len(rows))
	for _, row := range rows {
		out = append(out, storedRollbackReference(row))
	}
	return out, nil
}

func (s *Store) AbandonRollbackReferences(ctx context.Context, name, version, actor string, now time.Time) error {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return err
	}
	return s.secretQ.AbandonSecretRollbackReferences(ctx, db.AbandonSecretRollbackReferencesParams{
		Name: name, Version: number, AbandonedBy: actor, AbandonedAt: pgtype.Timestamptz{Time: now.UTC(), Valid: true},
	})
}

func (s *Store) DeleteVersion(ctx context.Context, name, version string, now time.Time) error {
	number, err := postgresSecretVersionNumber(version)
	if err != nil {
		return err
	}
	_, err = s.secretQ.DeleteSecretVersion(ctx, db.DeleteSecretVersionParams{Name: name, Version: number, Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return secrets.ErrVersionReferenced
	}
	return err
}

func storedRollbackReference(row db.SecretRollbackReference) secrets.StoredRollbackReference {
	reference := secrets.StoredRollbackReference{
		RollbackReferenceMetadata: secrets.RollbackReferenceMetadata{
			ID: row.ID, Reference: "remotr:" + row.Name + "@" + strconv.FormatInt(row.Version, 10), Fingerprint: row.Fingerprint,
			ResourceAddress: row.ResourceAddress, ArtifactDigest: row.ArtifactDigest, Attempt: int(row.Attempt),
			CreatedAt: pgTime(row.CreatedAt), ExpiresAt: pgTime(row.ExpiresAt), Status: secrets.RollbackReferenceStatus(row.Status),
			AbandonedAt: optionalPGTime(row.AbandonedAt), AbandonedBy: row.AbandonedBy,
		},
		Name: row.Name, Version: strconv.FormatInt(row.Version, 10),
	}
	return reference
}

func storedSecretVersion(envelopeJSON []byte, createdAt pgtype.Timestamptz, createdBy string, active bool, activationGeneration int64, activatedAt pgtype.Timestamptz, activatedBy string, revokedAt pgtype.Timestamptz, revokedBy string, rolloutsJSON []byte, queryErr error) (secrets.StoredVersion, error) {
	if errors.Is(queryErr, pgx.ErrNoRows) {
		return secrets.StoredVersion{}, secrets.ErrVersionNotFound
	}
	if queryErr != nil {
		return secrets.StoredVersion{}, queryErr
	}
	var record secrets.EncryptedRecord
	if err := json.Unmarshal(envelopeJSON, &record); err != nil {
		return secrets.StoredVersion{}, fmt.Errorf("decode stored secret envelope: %w", err)
	}
	var rollouts []secrets.RolloutBinding
	if len(rolloutsJSON) > 0 {
		if err := json.Unmarshal(rolloutsJSON, &rollouts); err != nil {
			return secrets.StoredVersion{}, fmt.Errorf("decode stored secret rollouts: %w", err)
		}
	}
	if activationGeneration < 0 {
		return secrets.StoredVersion{}, fmt.Errorf("invalid stored activation generation")
	}
	return secrets.StoredVersion{
		Record: record, CreatedAt: pgTime(createdAt), CreatedBy: createdBy, Active: active,
		ActivationGeneration: uint64(activationGeneration), ActivatedAt: optionalPGTime(activatedAt), ActivatedBy: activatedBy,
		RevokedAt: optionalPGTime(revokedAt), RevokedBy: revokedBy, Rollouts: rollouts,
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
