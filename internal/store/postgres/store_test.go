package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type fakeQuerier struct {
	byID     map[string]db.Endpoint
	byFP     map[string]db.Endpoint
	listRows []db.Endpoint
}

func (f *fakeQuerier) GetEndpointByID(_ context.Context, id string) (db.Endpoint, error) {
	row, ok := f.byID[id]
	if !ok {
		return db.Endpoint{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeQuerier) GetEndpointByFingerprint(_ context.Context, fp pgtype.Text) (db.Endpoint, error) {
	if !fp.Valid {
		return db.Endpoint{}, pgx.ErrNoRows
	}
	row, ok := f.byFP[fp.String]
	if !ok {
		return db.Endpoint{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeQuerier) EnsureFleet(context.Context, string) error { return nil }
func (f *fakeQuerier) RegisterEndpoint(context.Context, db.RegisterEndpointParams) (db.Endpoint, error) {
	return db.Endpoint{}, nil
}
func (f *fakeQuerier) BindFingerprint(context.Context, db.BindFingerprintParams) (db.Endpoint, error) {
	return db.Endpoint{}, nil
}
func (f *fakeQuerier) ListEndpoints(context.Context) ([]db.Endpoint, error) {
	return f.listRows, nil
}
func (f *fakeQuerier) DeleteEndpoint(_ context.Context, id string) (int64, error) {
	if _, ok := f.byID[id]; !ok {
		return 0, nil
	}
	delete(f.byID, id)
	return 1, nil
}
func (f *fakeQuerier) ListEndpointLabels(context.Context) ([]db.ListEndpointLabelsRow, error) {
	return nil, nil
}
func (f *fakeQuerier) ListEndpointLabelsForEndpoint(context.Context, string) ([]db.ListEndpointLabelsForEndpointRow, error) {
	return nil, nil
}
func (f *fakeQuerier) GetLatestDriftReport(context.Context, string) (db.DriftReport, error) {
	return db.DriftReport{}, pgx.ErrNoRows
}
func (f *fakeQuerier) CreateEnrollmentToken(context.Context, db.CreateEnrollmentTokenParams) (db.EnrollmentToken, error) {
	return db.EnrollmentToken{}, nil
}
func (f *fakeQuerier) ListEnrollmentTokens(context.Context) ([]db.EnrollmentToken, error) {
	return nil, nil
}
func (f *fakeQuerier) RevokeEnrollmentToken(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeQuerier) ConsumeEnrollmentToken(context.Context, string) (db.EnrollmentToken, error) {
	return db.EnrollmentToken{}, pgx.ErrNoRows
}
func (f *fakeQuerier) CreateDeploymentToken(context.Context, db.CreateDeploymentTokenParams) (db.DeploymentToken, error) {
	return db.DeploymentToken{}, nil
}
func (f *fakeQuerier) ListDeploymentTokens(context.Context) ([]db.ListDeploymentTokensRow, error) {
	return nil, nil
}
func (f *fakeQuerier) GetDeploymentTokenByLabel(context.Context, string) (db.DeploymentToken, error) {
	return db.DeploymentToken{}, pgx.ErrNoRows
}
func (f *fakeQuerier) GetDeploymentTokenByID(context.Context, pgtype.UUID) (db.DeploymentToken, error) {
	return db.DeploymentToken{}, pgx.ErrNoRows
}
func (f *fakeQuerier) RevokeDeploymentToken(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeQuerier) TouchDeploymentTokenUsed(context.Context, pgtype.UUID) error { return nil }
func (f *fakeQuerier) GetFleetSettings(context.Context, string) (db.FleetSetting, error) {
	return db.FleetSetting{}, pgx.ErrNoRows
}
func (f *fakeQuerier) UpsertFleetSettings(context.Context, db.UpsertFleetSettingsParams) (db.FleetSetting, error) {
	return db.FleetSetting{}, nil
}
func (f *fakeQuerier) RegisterOperatorCredential(context.Context, string) (db.OperatorCredential, error) {
	return db.OperatorCredential{}, nil
}
func (f *fakeQuerier) IsOperatorCredential(context.Context, string) (string, error) {
	return "", pgx.ErrNoRows
}
func (f *fakeQuerier) ListOperatorCredentials(context.Context) ([]db.OperatorCredential, error) {
	return nil, nil
}
func (f *fakeQuerier) CountOperatorCredentials(context.Context) (int64, error) { return 0, nil }
func (f *fakeQuerier) UpsertEndpointLabel(context.Context, db.UpsertEndpointLabelParams) error {
	return nil
}
func (f *fakeQuerier) InsertDriftReport(context.Context, db.InsertDriftReportParams) error {
	return nil
}
func (f *fakeQuerier) InsertApplyFailure(context.Context, db.InsertApplyFailureParams) error {
	return nil
}
func (f *fakeQuerier) GetServerSetting(context.Context, string) (string, error) {
	return "", pgx.ErrNoRows
}
func (f *fakeQuerier) UpsertServerSetting(context.Context, db.UpsertServerSettingParams) error {
	return nil
}

func TestStore_EndpointByID_registryInterface(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	fake := &fakeQuerier{
		byID: map[string]db.Endpoint{
			id.String(): {
				ID:    id.String(),
				Fleet: "test-fleet",
			},
		},
	}
	s := NewFromQueries(fake)

	ep, ok := s.EndpointByID(id.String())
	if !ok {
		t.Fatal("expected endpoint")
	}
	if ep.Fleet != "test-fleet" {
		t.Fatalf("fleet = %q", ep.Fleet)
	}

	_, ok = s.EndpointByID("00000000-0000-0000-0000-000000000000")
	if ok {
		t.Fatal("expected missing endpoint")
	}
}

func TestStore_EndpointByCertFingerprint(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	fp := "sha256:deadbeef"
	row := db.Endpoint{
		ID:              id.String(),
		Fleet:           "eng",
		CertFingerprint: pgtype.Text{String: fp, Valid: true},
	}
	fake := &fakeQuerier{
		byID: map[string]db.Endpoint{id.String(): row},
		byFP: map[string]db.Endpoint{fp: row},
	}
	s := NewFromQueries(fake)

	ep, ok := s.EndpointByCertFingerprint(fp)
	if !ok {
		t.Fatal("expected endpoint")
	}
	if ep.ID != id.String() {
		t.Fatalf("id = %q", ep.ID)
	}
}

func TestStore_DeleteEndpoint(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	fake := &fakeQuerier{
		byID: map[string]db.Endpoint{
			id.String(): {ID: id.String(), Fleet: "demo"},
		},
	}
	s := NewFromQueries(fake)

	ok, err := s.DeleteEndpoint(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected deleted")
	}
	if _, exists := fake.byID[id.String()]; exists {
		t.Fatal("endpoint still present")
	}

	ok, err = s.DeleteEndpoint(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected not found on second delete")
	}
}

func TestSetRemediationPolicy_rejectsUnknown(t *testing.T) {
	s := NewFromQueries(&fakeQuerier{})
	err := s.SetRemediationPolicy(context.Background(), "demo", "enforce")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStore_RegisterEndpoint_rejectsInvalidID(t *testing.T) {
	s := NewFromQueries(&fakeQuerier{})
	_, err := s.RegisterEndpoint(context.Background(), "bad_endpoint_id", "engineering", "fp")
	if err == nil {
		t.Fatal("expected error for invalid endpoint id")
	}
}

func TestConsumeEnrollmentToken_unavailable(t *testing.T) {
	s := NewFromQueries(&fakeQuerier{})
	_, err := s.ConsumeEnrollmentToken(context.Background(), "missing")
	if err != ErrEnrollmentTokenUnavailable {
		t.Fatalf("err = %v", err)
	}
}
