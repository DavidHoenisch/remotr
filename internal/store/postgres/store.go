package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/deploytoken"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// Store persists server registry data in Postgres and implements registry.Registry.
type Store struct {
	q Querier
}

// New opens a pool and returns a Store. Caller must run schema migration before use.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return NewFromPool(pool), nil
}

// NewFromPool wraps an existing pgx pool (for tests and wiring).
func NewFromPool(pool *pgxpool.Pool) *Store {
	return &Store{q: db.New(pool)}
}

// NewFromQueries wraps generated queries (for unit tests with fakes).
func NewFromQueries(q Querier) *Store {
	return &Store{q: q}
}

var _ registry.Registry = (*Store)(nil)

func (s *Store) EndpointByID(id string) (registry.Endpoint, bool) {
	ep, err := s.endpointByID(context.Background(), id)
	if err != nil {
		return registry.Endpoint{}, false
	}
	return ep, true
}

func (s *Store) EndpointByCertFingerprint(fp string) (registry.Endpoint, bool) {
	ep, err := s.endpointByFingerprint(context.Background(), fp)
	if err != nil {
		return registry.Endpoint{}, false
	}
	return ep, true
}

func (s *Store) endpointByID(ctx context.Context, id string) (registry.Endpoint, error) {
	uid, err := uuidFromString(id)
	if err != nil {
		return registry.Endpoint{}, err
	}
	row, err := s.q.GetEndpointByID(ctx, uid)
	if err != nil {
		return registry.Endpoint{}, err
	}
	return endpointFromRow(row)
}

func (s *Store) endpointByFingerprint(ctx context.Context, fp string) (registry.Endpoint, error) {
	row, err := s.q.GetEndpointByFingerprint(ctx, textFingerprint(fp))
	if err != nil {
		return registry.Endpoint{}, err
	}
	return endpointFromRow(row)
}

// RegisterEndpoint upserts an endpoint row. Fleet is created with default remediation auto if missing.
func (s *Store) RegisterEndpoint(ctx context.Context, id, fleet, certFingerprint string) (registry.Endpoint, error) {
	if err := s.ensureFleet(ctx, fleet); err != nil {
		return registry.Endpoint{}, err
	}
	uid, err := uuidFromString(id)
	if err != nil {
		return registry.Endpoint{}, err
	}
	row, err := s.q.RegisterEndpoint(ctx, db.RegisterEndpointParams{
		ID:              uid,
		Fleet:           fleet,
		CertFingerprint: textFingerprint(certFingerprint),
	})
	if err != nil {
		return registry.Endpoint{}, err
	}
	return endpointFromRow(row)
}

// BindFingerprint associates a certificate fingerprint with an enrolled endpoint.
func (s *Store) BindFingerprint(ctx context.Context, id, fingerprint string) (registry.Endpoint, error) {
	uid, err := uuidFromString(id)
	if err != nil {
		return registry.Endpoint{}, err
	}
	row, err := s.q.BindFingerprint(ctx, db.BindFingerprintParams{
		ID:              uid,
		CertFingerprint: textFingerprint(fingerprint),
	})
	if err != nil {
		return registry.Endpoint{}, err
	}
	return endpointFromRow(row)
}

// ListEndpoints returns all enrolled endpoints with labels attached.
func (s *Store) ListEndpoints(ctx context.Context) ([]registry.Endpoint, error) {
	rows, err := s.q.ListEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	labelMap, err := s.endpointLabelsMap(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]registry.Endpoint, 0, len(rows))
	for _, row := range rows {
		ep, err := endpointFromRow(row)
		if err != nil {
			return nil, err
		}
		ep.Labels = labelMap[ep.ID]
		out = append(out, ep)
	}
	return out, nil
}

// GetEndpoint returns one endpoint with labels and latest drift summary.
func (s *Store) GetEndpoint(ctx context.Context, id string) (registry.Endpoint, bool, error) {
	uid, err := uuidFromString(id)
	if err != nil {
		return registry.Endpoint{}, false, err
	}
	row, err := s.q.GetEndpointByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.Endpoint{}, false, nil
		}
		return registry.Endpoint{}, false, err
	}
	ep, err := endpointFromRow(row)
	if err != nil {
		return registry.Endpoint{}, false, err
	}
	ep.Labels, err = s.labelsForEndpoint(ctx, uid)
	if err != nil {
		return registry.Endpoint{}, false, err
	}
	drift, err := s.q.GetLatestDriftReport(ctx, uid)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return registry.Endpoint{}, false, err
		}
	} else if drift.ReportedAt.Valid {
		ep.LastDrift = &registry.DriftSummary{
			ReleaseRef: drift.ReleaseRef,
			Digest:     drift.Digest,
			ReportedAt: drift.ReportedAt.Time,
		}
	}
	return ep, true, nil
}

func (s *Store) endpointLabelsMap(ctx context.Context) (map[string]map[string]string, error) {
	rows, err := s.q.ListEndpointLabels(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]string)
	for _, row := range rows {
		id, err := uuidString(row.EndpointID)
		if err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = make(map[string]string)
		}
		out[id][row.Key] = row.Value
	}
	return out, nil
}

func (s *Store) labelsForEndpoint(ctx context.Context, endpointID pgtype.UUID) (map[string]string, error) {
	rows, err := s.q.ListEndpointLabelsForEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

// EnrollmentToken is a pending one-time enrollment secret.
type EnrollmentToken struct {
	Token     string
	Fleet     string
	ExpiresAt time.Time
}

// CreateEnrollmentToken stores a new token for the given fleet.
func (s *Store) CreateEnrollmentToken(ctx context.Context, token, fleet string, expiresAt time.Time) (EnrollmentToken, error) {
	if err := s.ensureFleet(ctx, fleet); err != nil {
		return EnrollmentToken{}, err
	}
	row, err := s.q.CreateEnrollmentToken(ctx, db.CreateEnrollmentTokenParams{
		Token: token,
		Fleet: fleet,
		ExpiresAt: pgtype.Timestamptz{
			Time:  expiresAt,
			Valid: true,
		},
	})
	if err != nil {
		return EnrollmentToken{}, err
	}
	return enrollmentTokenFromRow(row)
}

// ListEnrollmentTokens returns active (not consumed, not revoked) tokens.
func (s *Store) ListEnrollmentTokens(ctx context.Context) ([]EnrollmentToken, error) {
	rows, err := s.q.ListEnrollmentTokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EnrollmentToken, 0, len(rows))
	for _, row := range rows {
		tok, err := enrollmentTokenFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, nil
}

// RevokeEnrollmentToken invalidates an unused token. Returns false if no active token matched.
func (s *Store) RevokeEnrollmentToken(ctx context.Context, token string) (bool, error) {
	n, err := s.q.RevokeEnrollmentToken(ctx, token)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ConsumeEnrollmentToken marks a token used (one-time). Returns ErrEnrollmentTokenUnavailable if
// the token is missing, expired, revoked, or already consumed.
func (s *Store) ConsumeEnrollmentToken(ctx context.Context, token string) (EnrollmentToken, error) {
	row, err := s.q.ConsumeEnrollmentToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnrollmentToken{}, ErrEnrollmentTokenUnavailable
		}
		return EnrollmentToken{}, err
	}
	return enrollmentTokenFromRow(row)
}

// ErrEnrollmentTokenUnavailable is returned when a token cannot be consumed.
var ErrEnrollmentTokenUnavailable = errors.New("enrollment token unavailable")

// ErrDeploymentTokenLabelTaken is returned when a deployment token label already exists.
var ErrDeploymentTokenLabelTaken = registry.ErrDeploymentTokenLabelTaken

// ErrDeploymentTokenNotFound is returned when no deployment token matches the label.
var ErrDeploymentTokenNotFound = errors.New("deployment token not found")

// CreateDeploymentToken stores a hashed reusable deployment token and returns the raw secret once.
func (s *Store) CreateDeploymentToken(ctx context.Context, label, fleet string, expiresAt time.Time) (registry.DeploymentToken, string, error) {
	if err := deploytoken.ValidateLabel(label); err != nil {
		return registry.DeploymentToken{}, "", err
	}
	if err := s.ensureFleet(ctx, fleet); err != nil {
		return registry.DeploymentToken{}, "", err
	}

	raw, id, err := deploytoken.Issue()
	if err != nil {
		return registry.DeploymentToken{}, "", err
	}
	_, secret, err := deploytoken.Parse(raw)
	if err != nil {
		return registry.DeploymentToken{}, "", err
	}
	hash, err := deploytoken.HashSecret(secret)
	if err != nil {
		return registry.DeploymentToken{}, "", err
	}

	row, err := s.q.CreateDeploymentToken(ctx, db.CreateDeploymentTokenParams{
		ID:         pgtype.UUID{Bytes: id, Valid: true},
		Label:      label,
		Fleet:      fleet,
		SecretHash: hash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return registry.DeploymentToken{}, "", registry.ErrDeploymentTokenLabelTaken
		}
		return registry.DeploymentToken{}, "", err
	}

	tok, err := deploymentTokenFromRow(row)
	if err != nil {
		return registry.DeploymentToken{}, "", err
	}
	return tok, raw, nil
}

// ListDeploymentTokens returns deployment token metadata (never secrets).
func (s *Store) ListDeploymentTokens(ctx context.Context) ([]registry.DeploymentToken, error) {
	rows, err := s.q.ListDeploymentTokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]registry.DeploymentToken, 0, len(rows))
	for _, row := range rows {
		tok, err := deploymentTokenFromListRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, nil
}

// GetDeploymentTokenByLabel returns metadata for a deployment token identified by label.
func (s *Store) GetDeploymentTokenByLabel(ctx context.Context, label string) (registry.DeploymentToken, error) {
	row, err := s.q.GetDeploymentTokenByLabel(ctx, label)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.DeploymentToken{}, ErrDeploymentTokenNotFound
		}
		return registry.DeploymentToken{}, err
	}
	return deploymentTokenFromRow(row)
}

// RevokeDeploymentToken invalidates an active deployment token by label.
func (s *Store) RevokeDeploymentToken(ctx context.Context, label string) (bool, error) {
	n, err := s.q.RevokeDeploymentToken(ctx, label)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// RedeemDeploymentToken validates a reusable deployment token without consuming it.
func (s *Store) RedeemDeploymentToken(ctx context.Context, presented string) (string, bool) {
	id, secret, err := deploytoken.Parse(presented)
	if err != nil {
		return "", false
	}
	row, err := s.q.GetDeploymentTokenByID(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return "", false
	}
	if !deploymentTokenActive(row) {
		return "", false
	}
	if !deploytoken.VerifySecret(row.SecretHash, secret) {
		return "", false
	}
	_ = s.q.TouchDeploymentTokenUsed(ctx, pgtype.UUID{Bytes: id, Valid: true})
	return row.Fleet, true
}

// RemediationPolicy returns the fleet remediation policy (auto or report).
func (s *Store) RemediationPolicy(ctx context.Context, fleet string) (string, error) {
	row, err := s.q.GetFleetSettings(ctx, fleet)
	if err != nil {
		return "", err
	}
	return row.RemediationPolicy, nil
}

// SetRemediationPolicy upserts fleet remediation policy.
func (s *Store) SetRemediationPolicy(ctx context.Context, fleet, policy string) error {
	if policy != RemediationAuto && policy != RemediationReport {
		return fmt.Errorf("invalid remediation policy %q", policy)
	}
	_, err := s.q.UpsertFleetSettings(ctx, db.UpsertFleetSettingsParams{
		Fleet:             fleet,
		RemediationPolicy: policy,
	})
	return err
}

func (s *Store) ensureFleet(ctx context.Context, fleet string) error {
	return s.q.EnsureFleet(ctx, fleet)
}

const releaseRefSettingKey = "release_ref"

// GetReleaseRef returns the persisted global release ref, or empty if unset.
func (s *Store) GetReleaseRef(ctx context.Context) (string, error) {
	ref, err := s.q.GetServerSetting(ctx, releaseRefSettingKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return ref, nil
}

// SetReleaseRef persists the global release ref served to agents on sync.
func (s *Store) SetReleaseRef(ctx context.Context, ref string) error {
	return s.q.UpsertServerSetting(ctx, db.UpsertServerSettingParams{
		Key:   releaseRefSettingKey,
		Value: ref,
	})
}

// UpsertEndpointLabels stores endpoint inventory labels reported at sync.
func (s *Store) UpsertEndpointLabels(ctx context.Context, endpointID string, labels map[string]string) error {
	if len(labels) == 0 {
		return nil
	}
	uid, err := uuidFromString(endpointID)
	if err != nil {
		return err
	}
	for k, v := range labels {
		if err := s.q.UpsertEndpointLabel(ctx, db.UpsertEndpointLabelParams{
			EndpointID: uid,
			Key:        k,
			Value:      v,
		}); err != nil {
			return err
		}
	}
	return nil
}

// InsertDriftReport records agent-reported drift telemetry.
func (s *Store) InsertDriftReport(ctx context.Context, endpointID, releaseRef, digest string, reportJSON []byte) error {
	uid, err := uuidFromString(endpointID)
	if err != nil {
		return err
	}
	return s.q.InsertDriftReport(ctx, db.InsertDriftReportParams{
		ID:         newUUID(),
		EndpointID: uid,
		ReleaseRef: releaseRef,
		Digest:     digest,
		ReportJson: reportJSON,
	})
}

// InsertApplyFailure records the latest apply failure reported at sync.
func (s *Store) InsertApplyFailure(ctx context.Context, endpointID, releaseRef, resourceAddress, message string) error {
	uid, err := uuidFromString(endpointID)
	if err != nil {
		return err
	}
	return s.q.InsertApplyFailure(ctx, db.InsertApplyFailureParams{
		ID:              newUUID(),
		EndpointID:      uid,
		ReleaseRef:      releaseRef,
		ResourceAddress: resourceAddress,
		Message:         message,
	})
}

func newUUID() pgtype.UUID {
	id := uuid.New()
	return pgtype.UUID{Bytes: id, Valid: true}
}

// RegisterOperatorCredential stores an active operator client certificate fingerprint.
func (s *Store) RegisterOperatorCredential(ctx context.Context, fingerprint string) error {
	_, err := s.q.RegisterOperatorCredential(ctx, fingerprint)
	return err
}

// IsOperatorCredential reports whether fingerprint belongs to an active operator.
func (s *Store) IsOperatorCredential(ctx context.Context, fingerprint string) bool {
	_, err := s.q.IsOperatorCredential(ctx, fingerprint)
	return err == nil
}

// ListOperatorCredentials returns active operator certificate fingerprints.
func (s *Store) ListOperatorCredentials(ctx context.Context) ([]registry.OperatorCredential, error) {
	rows, err := s.q.ListOperatorCredentials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]registry.OperatorCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, registry.OperatorCredential{CertFingerprint: row.CertFingerprint})
	}
	return out, nil
}

// HasOperators reports whether any active operator credentials exist.
func (s *Store) HasOperators(ctx context.Context) (bool, error) {
	n, err := s.q.CountOperatorCredentials(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func enrollmentTokenFromRow(row db.EnrollmentToken) (EnrollmentToken, error) {
	if !row.ExpiresAt.Valid {
		return EnrollmentToken{}, errors.New("enrollment token missing expires_at")
	}
	return EnrollmentToken{
		Token:     row.Token,
		Fleet:     row.Fleet,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

func deploymentTokenFromRow(row db.DeploymentToken) (registry.DeploymentToken, error) {
	id, err := uuidString(row.ID)
	if err != nil {
		return registry.DeploymentToken{}, err
	}
	if !row.ExpiresAt.Valid {
		return registry.DeploymentToken{}, errors.New("deployment token missing expires_at")
	}
	if !row.CreatedAt.Valid {
		return registry.DeploymentToken{}, errors.New("deployment token missing created_at")
	}
	return registry.DeploymentToken{
		ID:         id,
		Label:      row.Label,
		Fleet:      row.Fleet,
		ExpiresAt:  row.ExpiresAt.Time,
		CreatedAt:  row.CreatedAt.Time,
		RevokedAt:  timestamptzPtr(row.RevokedAt),
		LastUsedAt: timestamptzPtr(row.LastUsedAt),
	}, nil
}

func deploymentTokenFromListRow(row db.ListDeploymentTokensRow) (registry.DeploymentToken, error) {
	id, err := uuidString(row.ID)
	if err != nil {
		return registry.DeploymentToken{}, err
	}
	if !row.ExpiresAt.Valid {
		return registry.DeploymentToken{}, errors.New("deployment token missing expires_at")
	}
	if !row.CreatedAt.Valid {
		return registry.DeploymentToken{}, errors.New("deployment token missing created_at")
	}
	return registry.DeploymentToken{
		ID:         id,
		Label:      row.Label,
		Fleet:      row.Fleet,
		ExpiresAt:  row.ExpiresAt.Time,
		CreatedAt:  row.CreatedAt.Time,
		RevokedAt:  timestamptzPtr(row.RevokedAt),
		LastUsedAt: timestamptzPtr(row.LastUsedAt),
	}, nil
}

func deploymentTokenActive(row db.DeploymentToken) bool {
	if row.RevokedAt.Valid {
		return false
	}
	if !row.ExpiresAt.Valid || row.ExpiresAt.Time.Before(time.Now()) {
		return false
	}
	return true
}

func timestamptzPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
