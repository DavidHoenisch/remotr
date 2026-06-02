package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// Querier is the sqlc-generated query surface used by Store (enables fakes in tests).
type Querier interface {
	EnsureFleet(ctx context.Context, fleet string) error
	RegisterEndpoint(ctx context.Context, arg db.RegisterEndpointParams) (db.Endpoint, error)
	GetEndpointByID(ctx context.Context, id string) (db.Endpoint, error)
	GetEndpointByFingerprint(ctx context.Context, certFingerprint pgtype.Text) (db.Endpoint, error)
	BindFingerprint(ctx context.Context, arg db.BindFingerprintParams) (db.Endpoint, error)
	ListEndpoints(ctx context.Context) ([]db.Endpoint, error)
	DeleteEndpoint(ctx context.Context, id string) (int64, error)
	CreateEnrollmentToken(ctx context.Context, arg db.CreateEnrollmentTokenParams) (db.EnrollmentToken, error)
	ListEnrollmentTokens(ctx context.Context) ([]db.EnrollmentToken, error)
	RevokeEnrollmentToken(ctx context.Context, token string) (int64, error)
	ConsumeEnrollmentToken(ctx context.Context, token string) (db.EnrollmentToken, error)
	CreateDeploymentToken(ctx context.Context, arg db.CreateDeploymentTokenParams) (db.DeploymentToken, error)
	ListDeploymentTokens(ctx context.Context) ([]db.ListDeploymentTokensRow, error)
	GetDeploymentTokenByLabel(ctx context.Context, label string) (db.DeploymentToken, error)
	GetDeploymentTokenByID(ctx context.Context, id pgtype.UUID) (db.DeploymentToken, error)
	RevokeDeploymentToken(ctx context.Context, label string) (int64, error)
	TouchDeploymentTokenUsed(ctx context.Context, id pgtype.UUID) error
	GetFleetSettings(ctx context.Context, fleet string) (db.FleetSetting, error)
	UpsertFleetSettings(ctx context.Context, arg db.UpsertFleetSettingsParams) (db.FleetSetting, error)
	RegisterOperatorCredential(ctx context.Context, certFingerprint string) (db.OperatorCredential, error)
	IsOperatorCredential(ctx context.Context, certFingerprint string) (string, error)
	ListOperatorCredentials(ctx context.Context) ([]db.OperatorCredential, error)
	CountOperatorCredentials(ctx context.Context) (int64, error)
	UpsertEndpointLabel(ctx context.Context, arg db.UpsertEndpointLabelParams) error
	ListEndpointLabels(ctx context.Context) ([]db.ListEndpointLabelsRow, error)
	ListEndpointLabelsForEndpoint(ctx context.Context, endpointID string) ([]db.ListEndpointLabelsForEndpointRow, error)
	InsertDriftReport(ctx context.Context, arg db.InsertDriftReportParams) error
	GetLatestDriftReport(ctx context.Context, endpointID string) (db.DriftReport, error)
	InsertApplyFailure(ctx context.Context, arg db.InsertApplyFailureParams) error
	GetServerSetting(ctx context.Context, key string) (string, error)
	UpsertServerSetting(ctx context.Context, arg db.UpsertServerSettingParams) error
}

var _ Querier = (*db.Queries)(nil)
