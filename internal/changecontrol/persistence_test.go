package changecontrol

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-AEC-076: lifecycle state and its audit evidence remain authoritative
// after a new registry instance is constructed from the same state store.
func TestPersistentRegistryRestoresLifecycleState(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	paused, err := registry.Pause(request.ID, "operator")
	if err != nil || paused.AuthorizationState != AuthorizationPaused {
		t.Fatalf("pause: request=%+v err=%v", paused, err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	got, ok := restored.Get(request.ID)
	if !ok || got.AuthorizationState != AuthorizationPaused {
		t.Fatalf("restored request = %+v, exists=%t", got, ok)
	}
	if len(got.AuditHistory) != 3 || got.AuditHistory[2].Action != AuditPaused {
		t.Fatalf("restored audit history = %+v", got.AuditHistory)
	}
}

func TestPersistentRegistryRejectsUnclassifiedPredictedEffectCanary(t *testing.T) {
	const canary = "change-control-persistence-secret-canary"
	store := &memoryStateStore{}
	registry := newPersistenceTestRegistry(t, t.Context(), store, time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC), "request")
	_, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity,
			Provider: "nftables", PredictedEffects: []PredictedEffect{{
				Code: EffectResourceUpdate,
				Details: executor.SafeSummary{Fields: []executor.SafeField{{
					Path: "content", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: canary,
				}}},
			}},
		}},
	}, "creator")
	if err == nil {
		t.Fatal("unclassified predicted effect was accepted")
	}
	if bytes.Contains(store.payload, []byte(canary)) {
		t.Fatalf("unclassified effect reached durable state: %s", store.payload)
	}
}

// OS-AEC-077: terminal execution progress releases durable concurrency while
// retaining the durable attempt count across a registry restart.
func TestPersistentRegistryRestoresExecutionProgress(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "lease-1")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{AttemptLimit: 2, MaxConcurrency: 1}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	lease, issued, err := registry.IssueExecutionLease(request.ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued {
		t.Fatalf("issue first lease: lease=%+v issued=%t err=%v", lease, issued, err)
	}
	updated, err := registry.UpdateExecutionProgress(lease.ID, ProgressUpdate{
		State: ProgressAcknowledged,
		Evidence: RiskEvidence{
			WatchdogArmed: true, AuthenticatedSync: true,
		},
	})
	if err != nil || !updated.Completed {
		t.Fatalf("acknowledge lease: lease=%+v err=%v", updated, err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now, "lease-2")
	next, issued, err := restored.IssueExecutionLease(request.ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued || next.ID != "lease-2" || next.Attempt != 2 {
		t.Fatalf("issue after restart: lease=%+v issued=%t err=%v", next, issued, err)
	}
}

func TestPersistentRegistryLeasePayloadIsAcceptedByPostgresJSONB(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{rejectJSONNull: true}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "lease")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{AttemptLimit: 1, MaxConcurrency: 1}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	lease, issued, err := registry.IssueExecutionLease(request.ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued || lease.Attempt != 1 {
		t.Fatalf("issue lease: lease=%+v issued=%t err=%v", lease, issued, err)
	}
}

func TestPersistentRegistryRestoresCompletedLease(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "lease-1")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{AttemptLimit: 2, MaxConcurrency: 1}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	lease, issued, err := registry.IssueExecutionLease(request.ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued {
		t.Fatalf("issue first lease: lease=%+v issued=%t err=%v", lease, issued, err)
	}
	if err := registry.CompleteExecutionLease(lease.ID); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now, "lease-2")
	next, issued, err := restored.IssueExecutionLease(request.ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued || next.Attempt != 2 {
		t.Fatalf("issue after restart: lease=%+v issued=%t err=%v", next, issued, err)
	}
}

func TestPersistentRegistryRestoresTargetOutcome(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request")
	request := createPersistenceTestRequest(t, registry)
	if err := registry.RecordTargetOutcome(request.ID, TargetOutcome{EndpointID: "endpoint", State: OutcomeVerifiedSuccess}, "agent"); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	summary, err := restored.OutcomeSummary(request.ID)
	if err != nil || summary.VerifiedSuccessful != 1 || summary.NotSeen != 0 {
		t.Fatalf("restored outcome summary = %+v, err=%v", summary, err)
	}
	got, _ := restored.Get(request.ID)
	if len(got.AuditHistory) != 2 || got.AuditHistory[1].Action != AuditTargetOutcome {
		t.Fatalf("restored audit history = %+v", got.AuditHistory)
	}
}

func TestPersistentRegistryRestoresBaselinePromotion(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "baseline")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PromoteBaseline(request.ID, "base/firewall", "operator"); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	if !restored.BaselineAuthorizes("engineering", "base/firewall", "hash", "nftables", true) {
		t.Fatal("restored baseline did not authorize the exact resource hash")
	}
	got, _ := restored.Get(request.ID)
	if len(got.AuditHistory) != 3 || got.AuditHistory[2].Action != AuditBaselinePromoted {
		t.Fatalf("restored audit history = %+v", got.AuditHistory)
	}
}

func TestPersistentRegistryRestoresBaselineInvalidation(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "baseline")
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PromoteBaseline(request.ID, "base/firewall", "operator"); err != nil {
		t.Fatal(err)
	}
	invalidated, err := registry.InvalidateBaselines("engineering", "base/firewall", "changed", "operator")
	if err != nil || len(invalidated) != 1 || invalidated[0].Active() {
		t.Fatalf("invalidate baseline: baselines=%+v err=%v", invalidated, err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	if restored.BaselineAuthorizes("engineering", "base/firewall", "hash", "nftables", true) {
		t.Fatal("invalidated baseline became active after restart")
	}
}

func TestPersistentRegistryRestoresApprovalPolicy(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now)
	if err := registry.SetApprovalPolicy(ApprovalPolicy{Global: map[models.RiskClass]int{models.RiskDestructive: 1}}); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now, "request")
	requests, err := restored.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/disk", DesiredHash: "hash", Risk: models.RiskDestructive, Provider: "disk"}},
	}, "creator")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: requests=%+v err=%v", requests, err)
	}
	if requests[0].RequiredApprovals != 1 || requests[0].PolicyWarning != SingleOperatorDestructiveWarning {
		t.Fatalf("request policy = %+v", requests[0])
	}
}

// OS-AEC-078: the approval and rollout activation are one durable transition;
// there is no intermediate approved-but-pending state to expose after restart.
func TestPersistentRegistryAuthorizesInOneAtomicCommit(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout")
	request := createPersistenceTestRequest(t, registry)
	store.failOnSaveCall = store.saveCalls + 2
	store.saveErr = errors.New("second write must not occur")

	authorization, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42")
	if err != nil || authorization.ID != "rollout" {
		t.Fatalf("authorize rollout: authorization=%+v err=%v", authorization, err)
	}
	restored := newPersistenceTestRegistry(t, ctx, store, now)
	got, _ := restored.Get(request.ID)
	if got.AuthorizationState != AuthorizationActive || len(got.Approvals) != 1 {
		t.Fatalf("restored authorization = %+v", got)
	}
}

func TestPersistentRegistryCreatesBaselineAdoptionInOneAtomicCommit(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{failOnSaveCall: 2, saveErr: errors.New("second write must not occur")}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request")
	request, err := registry.CreateBaselineAdoption(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "operator")
	if err != nil || request.ID != "request" || len(request.AuditHistory) != 2 || request.AuditHistory[1].Action != AuditBaselineAdoption {
		t.Fatalf("baseline adoption = %+v, err=%v", request, err)
	}
	restored := newPersistenceTestRegistry(t, ctx, store, now)
	got, ok := restored.Get(request.ID)
	if !ok || len(got.AuditHistory) != 2 || got.AuditHistory[1].Action != AuditBaselineAdoption {
		t.Fatalf("restored adoption = %+v, exists=%t", got, ok)
	}
}

func TestPersistentRegistryRollsBackExceptionPromotionOnSaveFailure(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout", "baseline")
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets: []TargetEvidence{
			{EndpointID: "successful", Compatible: true, PreflightReady: true},
			{EndpointID: "blocked", Compatible: true, PreflightReady: true},
		},
		Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables", BaselineEligible: true}},
	}, "creator")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: requests=%+v err=%v", requests, err)
	}
	request := requests[0]
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordTargetOutcome(request.ID, TargetOutcome{EndpointID: "successful", State: OutcomeVerifiedSuccess}, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordTargetOutcome(request.ID, TargetOutcome{EndpointID: "blocked", State: OutcomeCapabilityBlocked}, "agent"); err != nil {
		t.Fatal(err)
	}
	store.saveErr = errors.New("database unavailable")
	if _, err := registry.PromoteBaselineWithOptions(request.ID, "base/firewall", "operator", BaselinePromotionOptions{AcknowledgeExceptions: true}); !IsPersistenceError(err) {
		t.Fatalf("promotion error = %v", err)
	}

	got, _ := registry.Get(request.ID)
	if len(got.AuditHistory) != 4 || registry.BaselineAuthorizes("engineering", "base/firewall", "hash", "nftables", true) {
		t.Fatalf("failed promotion changed in-memory state: %+v", got)
	}
	restored := newPersistenceTestRegistry(t, ctx, store, now)
	got, _ = restored.Get(request.ID)
	if len(got.AuditHistory) != 4 || restored.BaselineAuthorizes("engineering", "base/firewall", "hash", "nftables", true) {
		t.Fatalf("failed promotion changed durable state: %+v", got)
	}
}

// OS-AEC-079: startup fails closed without exposing storage diagnostics or
// attacker-controlled persisted fields.
func TestPersistentRegistryRejectsUnreadableStateWithSafeDiagnostic(t *testing.T) {
	tests := []struct {
		name   string
		store  *memoryStateStore
		target error
	}{
		{
			name:   "storage read failure",
			store:  &memoryStateStore{loadErr: errors.New("database storage-secret-canary")},
			target: ErrPersistence,
		},
		{
			name: "malformed payload",
			store: &memoryStateStore{
				payload: []byte(`{"version":1,"storage-secret-canary":"value"}`), revision: 1,
			},
			target: ErrInvalidPersistedState,
		},
		{
			name: "authorized request without rollout",
			store: &memoryStateStore{
				payload: []byte(`{"version":1,"requests":{"request":{"id":"request","authorization_state":"authorized"}}}`), revision: 1,
			},
			target: ErrInvalidPersistedState,
		},
		{
			name:   "payload without durable revision",
			store:  &memoryStateStore{payload: []byte(`{"version":1}`)},
			target: ErrInvalidPersistedState,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, err := NewPersistentRegistry(context.Background(), test.store, RegistryOptions{})
			if registry != nil || !errors.Is(err, test.target) {
				t.Fatalf("registry=%v error=%v", registry, err)
			}
			if bytes.Contains([]byte(err.Error()), []byte("storage-secret-canary")) {
				t.Fatalf("startup diagnostic leaked persisted detail: %v", err)
			}
		})
	}
}

func TestPersistentRegistryRestoresAutomaticPromotionPolicy(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "rollout")
	if err := registry.SetAutomaticPromotionPolicy("engineering", AutomaticPromotionPolicy{
		CanaryStages: []int{1}, MinimumSuccessful: 1, MaximumFailures: 0,
	}); err != nil {
		t.Fatal(err)
	}
	request := createPersistenceTestRequest(t, registry)
	if _, err := registry.AuthorizeRollout(request.ID, RolloutSpec{}, "approver", "CHG-42"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordTargetOutcome(request.ID, TargetOutcome{EndpointID: "endpoint", State: OutcomeVerifiedSuccess}, "agent"); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now, "baseline")
	baseline, err := restored.TryAutomaticBaselinePromotion(request.ID, "base/firewall")
	if err != nil || baseline.ID != "baseline" || baseline.AuthorizedBy != "system" {
		t.Fatalf("automatic promotion = %+v, err=%v", baseline, err)
	}
}

func TestPersistentRegistryRestoresBreakGlassAuthorization(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "break-glass")
	request := createPersistenceTestCanonicalBreakGlassRequest(t, registry)
	authorization, err := registry.CreateBreakGlass(BreakGlassSpec{
		ChangeRequestID: request.ID, EndpointIDs: []string{"endpoint"},
		Justification: "restore access", ExternalReference: "INC-42",
	}, "operator", "")
	if err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	used, err := restored.UseBreakGlass(authorization.ID, PreflightReport{ChangeRequestID: request.ID, EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"base/firewall": persistenceBreakGlassHash}})
	if err != nil || used.Attempts != 1 || len(used.AuditHistory) != 2 || used.AuditHistory[1].Action != AuditBreakGlassUsed {
		t.Fatalf("restored break glass = %+v, err=%v", used, err)
	}
}

func TestPersistentRegistryRestoresBreakGlassAttempt(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "break-glass")
	authorization := createPersistenceTestBreakGlass(t, registry)
	if _, err := registry.UseBreakGlass(authorization.ID, PreflightReport{ChangeRequestID: authorization.ChangeRequestID, EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"base/firewall": persistenceBreakGlassHash}}); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	revoked, err := restored.RevokeBreakGlass(authorization.ID, "operator")
	if err != nil || revoked.Attempts != 1 || len(revoked.AuditHistory) != 3 || revoked.AuditHistory[1].Action != AuditBreakGlassUsed {
		t.Fatalf("restored break-glass attempt = %+v, err=%v", revoked, err)
	}
}

func TestPersistentRegistryRestoresBreakGlassRevocation(t *testing.T) {
	ctx := context.Background()
	store := &memoryStateStore{}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := newPersistenceTestRegistry(t, ctx, store, now, "request", "break-glass")
	authorization := createPersistenceTestBreakGlass(t, registry)
	if _, err := registry.RevokeBreakGlass(authorization.ID, "operator"); err != nil {
		t.Fatal(err)
	}

	restored := newPersistenceTestRegistry(t, ctx, store, now)
	revoked, err := restored.RevokeBreakGlass(authorization.ID, "auditor")
	if err != nil || !revoked.Revoked || len(revoked.AuditHistory) != 3 || revoked.AuditHistory[1].ActorID != "operator" {
		t.Fatalf("restored revocation = %+v, err=%v", revoked, err)
	}
}

func newPersistenceTestRegistry(t *testing.T, ctx context.Context, store StateStore, now time.Time, ids ...string) *Registry {
	t.Helper()
	index := 0
	registry, err := NewPersistentRegistry(ctx, store, RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			if index >= len(ids) {
				return "unexpected-id"
			}
			id := ids[index]
			index++
			return id
		},
		CanBreakGlass: func(string, string, models.RiskClass) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func createPersistenceTestRequest(t *testing.T, registry *Registry) ChangeRequest {
	t.Helper()
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity,
			Provider: "nftables", BaselineEligible: true,
		}},
	}, "creator")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: requests=%+v err=%v", requests, err)
	}
	return requests[0]
}

func createPersistenceTestBreakGlass(t *testing.T, registry *Registry) BreakGlassAuthorization {
	t.Helper()
	request := createPersistenceTestCanonicalBreakGlassRequest(t, registry)
	authorization, err := registry.CreateBreakGlass(BreakGlassSpec{
		ChangeRequestID: request.ID, EndpointIDs: []string{"endpoint"},
		Justification: "restore access", ExternalReference: "INC-42",
	}, "operator", "")
	if err != nil {
		t.Fatal(err)
	}
	return authorization
}

const persistenceBreakGlassHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func createPersistenceTestCanonicalBreakGlassRequest(t *testing.T, registry *Registry) ChangeRequest {
	t.Helper()
	plan := FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact", HashContractVersion: 1,
		Targets: []TargetEvidence{{
			EndpointID: "endpoint", Compatible: true, PreflightReady: true,
			ResourcePreflights: []ResourcePreflightEvidence{{Address: "base/firewall", Ready: true}},
		}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: persistenceBreakGlassHash, Risk: models.RiskConnectivity,
			Provider: "firewall", ProviderRevision: "firewall-v1", RollbackClass: "transactional",
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "base/firewall", EffectiveHash: persistenceBreakGlassHash, Provider: "firewall",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "creator")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create canonical break-glass request: requests=%+v err=%v", requests, err)
	}
	return requests[0]
}

type memoryStateStore struct {
	mu             sync.Mutex
	payload        []byte
	revision       int64
	saveErr        error
	loadErr        error
	saveCalls      int
	failOnSaveCall int
	rejectJSONNull bool
}

func (s *memoryStateStore) LoadChangeControlState(context.Context) ([]byte, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.payload...), s.revision, s.loadErr
}

func (s *memoryStateStore) SaveChangeControlState(_ context.Context, expectedRevision int64, payload []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil && (s.failOnSaveCall == 0 || s.failOnSaveCall == s.saveCalls) {
		err := s.saveErr
		s.saveErr = nil
		return 0, err
	}
	if expectedRevision != s.revision {
		return 0, errors.New("revision conflict")
	}
	if s.rejectJSONNull && bytes.Contains(payload, []byte(`\u0000`)) {
		return 0, errors.New("Postgres JSONB rejects the null escape")
	}
	s.revision++
	s.payload = append([]byte(nil), payload...)
	return s.revision, nil
}
