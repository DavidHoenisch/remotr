package postgres

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type fakeQuerier struct {
	byID                map[string]db.Endpoint
	byFP                map[string]db.Endpoint
	listRows            []db.Endpoint
	fleetRows           []db.FleetSetting
	latestApplyFailure  db.ApplyFailure
	hasApplyFailure     bool
	insertedDrift       *db.InsertDriftReportParams
	insertedAudit       *db.InsertAuditEventParams
	completedDiagnostic *db.CompleteDiagnosticRequestParams
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
func (f *fakeQuerier) DeleteEndpointLabel(_ context.Context, arg db.DeleteEndpointLabelParams) (int64, error) {
	return 0, nil
}
func (f *fakeQuerier) GetLatestDriftReport(context.Context, string) (db.DriftReport, error) {
	return db.DriftReport{}, pgx.ErrNoRows
}
func (f *fakeQuerier) UpsertEndpointSystemInfo(context.Context, db.UpsertEndpointSystemInfoParams) error {
	return nil
}
func (f *fakeQuerier) GetEndpointSystemInfo(context.Context, string) (db.EndpointSystemInfo, error) {
	return db.EndpointSystemInfo{}, pgx.ErrNoRows
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
func (f *fakeQuerier) TouchDeploymentTokenUsed(context.Context, pgtype.UUID) error  { return nil }
func (f *fakeQuerier) GetFleetSettings(context.Context, string) (db.FleetSetting, error) {
	return db.FleetSetting{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListFleets(context.Context) ([]db.FleetSetting, error) {
	return f.fleetRows, nil
}
func (f *fakeQuerier) UpsertFleetSettings(context.Context, db.UpsertFleetSettingsParams) (db.FleetSetting, error) {
	return db.FleetSetting{}, nil
}
func (f *fakeQuerier) RegisterOperatorCredential(context.Context, db.RegisterOperatorCredentialParams) (db.OperatorCredential, error) {
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

func (f *fakeQuerier) InsertDriftReport(_ context.Context, params db.InsertDriftReportParams) error {
	f.insertedDrift = &params
	return nil
}
func (f *fakeQuerier) InsertApplyFailure(context.Context, db.InsertApplyFailureParams) error {
	return nil
}
func (f *fakeQuerier) InsertFirewallAuditReport(context.Context, db.InsertFirewallAuditReportParams) error {
	return nil
}
func (f *fakeQuerier) GetLatestFirewallAuditReport(context.Context, string) (db.FirewallAuditReport, error) {
	return db.FirewallAuditReport{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListFleetFirewallAuditReports(context.Context, string) ([]db.FirewallAuditReport, error) {
	return nil, nil
}
func (f *fakeQuerier) PruneOldFirewallAuditReports(context.Context, pgtype.Timestamptz) error {
	return nil
}
func (f *fakeQuerier) GetLatestApplyFailure(_ context.Context, endpointID string) (db.ApplyFailure, error) {
	if !f.hasApplyFailure || f.latestApplyFailure.EndpointID != endpointID {
		return db.ApplyFailure{}, pgx.ErrNoRows
	}
	return f.latestApplyFailure, nil
}

func TestInsertDriftReportRejectsInvalidClassifiedSummaryBeforeDatabase(t *testing.T) {
	const canary = "postgres-state-report-secret-canary"
	fq := &fakeQuerier{}
	store := &Store{q: fq}
	unsafe := registry.StateReportPayload{SchemaVersion: 7, Items: []registry.StateReportItem{{
		Address: "base/managed",
		DesiredSummary: executor.SafeSummary{Fields: []executor.SafeField{{
			Path: "content", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: canary,
		}}},
	}}}
	err := store.InsertDriftReport(t.Context(), "11111111-1111-1111-1111-111111111111", "release", "digest", unsafe)
	if err == nil || !strings.Contains(err.Error(), "invalid classified state report") {
		t.Fatalf("InsertDriftReport() error = %v", err)
	}
	if fq.insertedDrift != nil {
		t.Fatalf("unsafe report reached database query: %+v", fq.insertedDrift)
	}
}
func (f *fakeQuerier) GetServerSetting(context.Context, string) (string, error) {
	return "", pgx.ErrNoRows
}
func (f *fakeQuerier) UpsertServerSetting(context.Context, db.UpsertServerSettingParams) error {
	return nil
}
func (f *fakeQuerier) SetEndpointDesiredAgentVersion(context.Context, db.SetEndpointDesiredAgentVersionParams) (db.Endpoint, error) {
	return db.Endpoint{}, nil
}
func (f *fakeQuerier) SetFleetDesiredAgentVersion(context.Context, db.SetFleetDesiredAgentVersionParams) (int64, error) {
	return 0, nil
}
func (f *fakeQuerier) ClearEndpointDesiredAgentVersion(context.Context, string) (db.Endpoint, error) {
	return db.Endpoint{}, nil
}
func (f *fakeQuerier) UpdateEndpointAgentUpgradeReport(context.Context, db.UpdateEndpointAgentUpgradeReportParams) (db.Endpoint, error) {
	return db.Endpoint{}, nil
}
func (f *fakeQuerier) InsertAuditEvent(_ context.Context, params db.InsertAuditEventParams) error {
	f.insertedAudit = &params
	return nil
}
func (f *fakeQuerier) ListAuditEvents(context.Context, db.ListAuditEventsParams) ([]db.AuditEvent, error) {
	return nil, nil
}

func TestRecordAuditEventRejectsUnclassifiedDetailsBeforeDatabase(t *testing.T) {
	const canary = "postgres-audit-detail-secret-canary"
	fq := &fakeQuerier{}
	store := &Store{q: fq}
	err := store.RecordAuditEvent(t.Context(), audit.Event{
		Action: "test", Method: "POST", Path: "/v1/test", StatusCode: 200,
		Details: &executor.SafeSummary{Fields: []executor.SafeField{{
			Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: canary,
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid classified audit details") {
		t.Fatalf("RecordAuditEvent() error = %v", err)
	}
	if fq.insertedAudit != nil {
		t.Fatalf("unsafe audit event reached database query: %+v", fq.insertedAudit)
	}
}

func TestAuditEventClassifiedDetailsSurviveDurableReadAndLegacyCanaryIsDiscarded(t *testing.T) {
	present := true
	details := executor.SafeSummary{Fields: []executor.SafeField{{
		Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
	}}}
	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	row := db.AuditEvent{
		ID:         pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true},
		OccurredAt: pgtype.Timestamptz{Time: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC), Valid: true},
		ActorType:  audit.ActorOperator, Action: audit.ActionAdminGitSync,
		Method: "POST", Path: "/v1/admin/git-sync", StatusCode: 200, Details: encoded,
	}
	restored, err := auditEventFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Details == nil || restored.Details.String() != "secret=true" {
		t.Fatalf("restored classified audit details = %+v", restored.Details)
	}

	const canary = "legacy-audit-detail-secret-canary"
	row.Details = []byte(`{"secret":"` + canary + `"}`)
	legacy, err := auditEventFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Details != nil {
		t.Fatalf("legacy unclassified details survived durable read: %+v", legacy.Details)
	}
}
func (f *fakeQuerier) UpsertRBACRole(context.Context, db.UpsertRBACRoleParams) error { return nil }
func (f *fakeQuerier) ListRBACRoles(context.Context) ([]db.RbacRole, error)          { return nil, nil }
func (f *fakeQuerier) GetRBACRole(context.Context, string) (db.RbacRole, error) {
	return db.RbacRole{}, pgx.ErrNoRows
}
func (f *fakeQuerier) DeleteRBACRole(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeQuerier) ListRBACRulesForRole(context.Context, string) ([]db.RbacRule, error) {
	return nil, nil
}
func (f *fakeQuerier) InsertRBACRule(context.Context, db.InsertRBACRuleParams) (db.RbacRule, error) {
	return db.RbacRule{}, nil
}
func (f *fakeQuerier) DeleteRBACRule(context.Context, db.DeleteRBACRuleParams) (int64, error) {
	return 0, nil
}
func (f *fakeQuerier) ListOperatorRoleAssignments(context.Context) ([]db.OperatorRoleAssignment, error) {
	return nil, nil
}
func (f *fakeQuerier) ListOperatorRoleAssignmentsForOperator(context.Context, string) ([]string, error) {
	return nil, nil
}
func (f *fakeQuerier) ReplaceOperatorRoleAssignments(context.Context, string) error { return nil }
func (f *fakeQuerier) InsertOperatorRoleAssignment(context.Context, db.InsertOperatorRoleAssignmentParams) error {
	return nil
}
func (f *fakeQuerier) ListActiveOperators(context.Context) ([]db.OperatorCredential, error) {
	return nil, nil
}
func (f *fakeQuerier) UpdateEndpointCheckIn(_ context.Context, arg db.UpdateEndpointCheckInParams) error {
	row, ok := f.byID[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	row.LastSyncAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	row.LastSeenReleaseRef = arg.LastSeenReleaseRef
	row.LastSeenDigest = arg.LastSeenDigest
	f.byID[arg.ID] = row
	return nil
}
func (f *fakeQuerier) UpdateEndpointUsernames(_ context.Context, arg db.UpdateEndpointUsernamesParams) error {
	row, ok := f.byID[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	row.ReportedUsernames = arg.ReportedUsernames
	f.byID[arg.ID] = row
	return nil
}
func (f *fakeQuerier) GetCronLastRun(context.Context, db.GetCronLastRunParams) (db.CronLastRun, error) {
	return db.CronLastRun{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListCronLastRunsForEndpoint(context.Context, string) ([]db.CronLastRun, error) {
	return nil, nil
}
func (f *fakeQuerier) UpsertCronLastRun(context.Context, db.UpsertCronLastRunParams) error {
	return nil
}
func (f *fakeQuerier) InsertCronExecution(context.Context, db.InsertCronExecutionParams) error {
	return nil
}
func (f *fakeQuerier) ListCronExecutionsForEndpoint(context.Context, db.ListCronExecutionsForEndpointParams) ([]db.CronExecution, error) {
	return nil, nil
}
func (f *fakeQuerier) CreateAppPackage(context.Context, db.CreateAppPackageParams) (db.AppPackage, error) {
	return db.AppPackage{}, nil
}
func (f *fakeQuerier) GetAppPackage(context.Context, db.GetAppPackageParams) (db.AppPackage, error) {
	return db.AppPackage{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListAppPackages(context.Context, string) ([]db.AppPackage, error) {
	return nil, nil
}
func (f *fakeQuerier) DeleteAppPackage(context.Context, db.DeleteAppPackageParams) (int64, error) {
	return 0, nil
}
func (f *fakeQuerier) InsertDiagnosticRequest(context.Context, db.InsertDiagnosticRequestParams) (db.DiagnosticRequest, error) {
	return db.DiagnosticRequest{}, nil
}
func (f *fakeQuerier) GetDiagnosticRequest(context.Context, pgtype.UUID) (db.DiagnosticRequest, error) {
	return db.DiagnosticRequest{}, pgx.ErrNoRows
}
func (f *fakeQuerier) GetActiveDiagnosticRequestForEndpoint(context.Context, string) (db.DiagnosticRequest, error) {
	return db.DiagnosticRequest{}, pgx.ErrNoRows
}
func (f *fakeQuerier) MarkDiagnosticRequestDispatched(context.Context, pgtype.UUID) (db.DiagnosticRequest, error) {
	return db.DiagnosticRequest{}, nil
}
func (f *fakeQuerier) MarkDiagnosticRequestRunning(context.Context, pgtype.UUID) (db.DiagnosticRequest, error) {
	return db.DiagnosticRequest{}, nil
}
func (f *fakeQuerier) CompleteDiagnosticRequest(_ context.Context, params db.CompleteDiagnosticRequestParams) (db.DiagnosticRequest, error) {
	f.completedDiagnostic = &params
	return db.DiagnosticRequest{}, nil
}
func (f *fakeQuerier) ExpireDiagnosticRequests(context.Context) error { return nil }

func TestCompleteDiagnosticRequestRejectsUnclassifiedFailureBeforeDatabase(t *testing.T) {
	const canary = "diagnostic-failure-secret-canary"
	fq := &fakeQuerier{}
	store := &Store{q: fq}
	err := store.CompleteDiagnosticRequest(t.Context(), diagnostics.ResultPayload{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Status:    diagnostics.StatusFailed,
		Failure: &executor.SafeError{
			ReasonCode: "diagnostic_collection_failed", Operation: "diagnostic_collection",
			Details: executor.SafeSummary{Fields: []executor.SafeField{{
				Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: canary,
			}}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid classified diagnostic failure") {
		t.Fatalf("CompleteDiagnosticRequest() error = %v", err)
	}
	if fq.completedDiagnostic != nil {
		t.Fatalf("unsafe diagnostic failure reached database query: %+v", fq.completedDiagnostic)
	}
}

func TestCompleteDiagnosticRequestPersistsOnlyValidatedClassifiedFailure(t *testing.T) {
	present := true
	details, err := executor.NewSafeSummary([]executor.SafeField{{
		Path: "source.present", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
	}})
	if err != nil {
		t.Fatal(err)
	}
	failure := executor.NewSafeErrorWithDetails("diagnostic_collection_failed", "diagnostic_collection", nil, details)
	fq := &fakeQuerier{}
	store := &Store{q: fq}
	if err := store.CompleteDiagnosticRequest(t.Context(), diagnostics.ResultPayload{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Status:    diagnostics.StatusFailed,
		SHA256:    strings.Repeat("a", 64),
		SizeBytes: 42,
		Failure:   &failure,
	}); err != nil {
		t.Fatal(err)
	}
	if fq.completedDiagnostic == nil {
		t.Fatal("classified diagnostic failure did not reach persistence")
	}
	var restored executor.SafeError
	if err := json.Unmarshal([]byte(fq.completedDiagnostic.ErrorMessage), &restored); err != nil {
		t.Fatalf("persisted diagnostic failure is not classified JSON: %v", err)
	}
	if !reflect.DeepEqual(restored, failure) {
		t.Fatalf("persisted diagnostic failure = %#v, want %#v", restored, failure)
	}

	withoutFailure := &fakeQuerier{}
	store = &Store{q: withoutFailure}
	if err := store.CompleteDiagnosticRequest(t.Context(), diagnostics.ResultPayload{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Status:    "unexpected",
	}); err != nil {
		t.Fatal(err)
	}
	if withoutFailure.completedDiagnostic == nil || withoutFailure.completedDiagnostic.Status != diagnostics.StatusFailed || withoutFailure.completedDiagnostic.ErrorMessage != "" {
		t.Fatalf("unclassified status/failure persisted as %+v", withoutFailure.completedDiagnostic)
	}
}

func TestDiagnosticRequestFromRowRestoresOnlyClassifiedFailure(t *testing.T) {
	failure := executor.NewSafeError("diagnostic_collection_failed", "diagnostic_collection", nil)
	failureJSON, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}

	req, err := diagnosticRequestFromRow(db.DiagnosticRequest{
		SpecJson:     []byte(`{}`),
		ErrorMessage: string(failureJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Failure == nil || req.Failure.ReasonCode != failure.ReasonCode || req.Failure.Operation != failure.Operation || req.Failure.Canceled != failure.Canceled {
		t.Fatalf("classified failure = %+v, want %+v", req.Failure, failure)
	}

	const legacyCanary = "legacy-diagnostic-error-secret-canary"
	legacy, err := diagnosticRequestFromRow(db.DiagnosticRequest{
		SpecJson:     []byte(`{}`),
		ErrorMessage: legacyCanary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Failure != nil {
		t.Fatalf("legacy unclassified failure was restored: %+v", legacy.Failure)
	}
}

func (f *fakeQuerier) DeleteExpiredDiagnosticRequests(context.Context) ([]db.DiagnosticRequest, error) {
	return nil, nil
}
func (f *fakeQuerier) UpsertCompiledArtifactForFleet(context.Context, db.UpsertCompiledArtifactForFleetParams) (db.CompiledArtifact, error) {
	return db.CompiledArtifact{}, nil
}
func (f *fakeQuerier) UpsertCompiledArtifactForEndpoint(context.Context, db.UpsertCompiledArtifactForEndpointParams) (db.CompiledArtifact, error) {
	return db.CompiledArtifact{}, nil
}
func (f *fakeQuerier) GetCompiledArtifactForFleet(context.Context, db.GetCompiledArtifactForFleetParams) (db.GetCompiledArtifactForFleetRow, error) {
	return db.GetCompiledArtifactForFleetRow{}, pgx.ErrNoRows
}
func (f *fakeQuerier) GetCompiledArtifactForEndpoint(context.Context, db.GetCompiledArtifactForEndpointParams) (db.GetCompiledArtifactForEndpointRow, error) {
	return db.GetCompiledArtifactForEndpointRow{}, pgx.ErrNoRows
}
func (f *fakeQuerier) PruneOldCompiledArtifacts(context.Context, pgtype.Timestamptz) error {
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

func TestStore_ListFleets(t *testing.T) {
	s := NewFromQueries(&fakeQuerier{
		fleetRows: []db.FleetSetting{
			{Fleet: "engineering"},
			{Fleet: "platform"},
		},
	})

	fleetStore, ok := any(s).(interface {
		ListFleets(context.Context) ([]string, error)
	})
	if !ok {
		t.Fatal("store missing ListFleets")
	}
	fleets, err := fleetStore.ListFleets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(fleets) != 2 || fleets[0] != "engineering" || fleets[1] != "platform" {
		t.Fatalf("fleets = %+v", fleets)
	}
}

func TestStore_GetEndpoint_includesLatestApplyFailure(t *testing.T) {
	const endpointID = "phalanx-acae925c"
	reportedAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	fake := &fakeQuerier{
		byID: map[string]db.Endpoint{
			endpointID: {ID: endpointID, Fleet: "engineering"},
		},
		hasApplyFailure: true,
		latestApplyFailure: db.ApplyFailure{
			EndpointID:      endpointID,
			ReleaseRef:      "edf7176",
			ResourceAddress: "base-packages/true",
			Message:         "exit status 1",
			ReportedAt:      pgtype.Timestamptz{Time: reportedAt, Valid: true},
		},
	}
	s := NewFromQueries(fake)
	ep, ok, err := s.GetEndpoint(context.Background(), endpointID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected endpoint")
	}
	if ep.LastApplyFailure == nil {
		t.Fatal("expected last apply failure")
	}
	if ep.LastApplyFailure.ResourceAddress != "base-packages/true" {
		t.Fatalf("resource = %q", ep.LastApplyFailure.ResourceAddress)
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
