package changecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Task 1.3 compatibility baseline at the Change-control persistence seam. The
// expected file records the future migration disposition independently of the
// current version-1 decoder; task 4.7 will make enforcement honor it.
func TestLegacyPersistedPlanCompatibilityFixture(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "persisted-state-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	expectedRaw, err := os.ReadFile(filepath.Join("testdata", "persisted-state-v1.expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		RequestID          string `json:"request_id"`
		ResourceAddress    string `json:"resource_address"`
		CallerAuthoredHash string `json:"caller_authored_hash"`
		Visibility         string `json:"visibility"`
		Enforcement        string `json:"enforcement"`
		Replacement        string `json:"replacement"`
		Reason             string `json:"reason"`
	}
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatal(err)
	}
	if expected.Visibility != "visible" || expected.Enforcement != "non_enforcing" || expected.Replacement != "explicit_regeneration_required" || expected.Reason != "legacy_plan_has_no_canonical_hash_contract_version" {
		t.Fatalf("legacy migration disposition = %+v", expected)
	}

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	registry, err := NewPersistentRegistry(context.Background(), compatibilityStateStore{payload: payload, revision: 1}, RegistryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request, ok := registry.Get(expected.RequestID)
	if !ok {
		t.Fatalf("legacy request %q is not visible", expected.RequestID)
	}
	if len(request.Resources) != 1 || request.Resources[0].Address != expected.ResourceAddress || request.Resources[0].DesiredHash != expected.CallerAuthoredHash {
		t.Fatalf("legacy request plan = %+v", request.Resources)
	}
	if len(request.Resources[0].PredictedEffects) != 1 || request.Resources[0].PredictedEffects[0].Code != EffectLegacyUnclassified {
		t.Fatalf("legacy predicted effects = %+v", request.Resources[0].PredictedEffects)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("caller supplied effect")) {
		t.Fatalf("legacy free-form effect survived migration: %s", encoded)
	}
	if request.AuthorizationState != AuthorizationActive {
		t.Fatalf("fixture must preserve the formerly enforcing state, got %q", request.AuthorizationState)
	}
	if request.LegacyMigration == nil || request.LegacyMigration.Enforcement != expected.Enforcement || request.LegacyMigration.Replacement != expected.Replacement || request.LegacyMigration.Reason != expected.Reason {
		t.Fatalf("legacy migration status = %+v", request.LegacyMigration)
	}
	if rollout := registry.rollouts[expected.RequestID]; rollout.LegacyMigration == nil || rollout.LegacyMigration.Enforcement != expected.Enforcement {
		t.Fatalf("legacy rollout migration status = %+v", rollout.LegacyMigration)
	}
	if baseline := registry.baselines[baselineKey("engineering", expected.ResourceAddress)]; baseline.LegacyMigration == nil || baseline.LegacyMigration.Enforcement != expected.Enforcement {
		t.Fatalf("legacy baseline migration status = %+v", baseline.LegacyMigration)
	}
	if registry.RolloutActive(expected.RequestID, now) {
		t.Fatal("legacy rollout remained enforcing after restore")
	}
	if _, issued, err := registry.IssueExecutionLease(expected.RequestID, PreflightReport{ChangeRequestID: expected.RequestID, EndpointID: "endpoint-legacy", Ready: true, ResourceHashes: map[string]string{expected.ResourceAddress: expected.CallerAuthoredHash}}); err != nil || issued {
		t.Fatalf("legacy execution lease issued=%t err=%v", issued, err)
	}
	if registry.BaselineAuthorizes("engineering", expected.ResourceAddress, expected.CallerAuthoredHash, "nftables", true) {
		t.Fatal("legacy baseline remained enforcing after restore")
	}
}

type compatibilityStateStore struct {
	payload  []byte
	revision int64
}

func (s compatibilityStateStore) LoadChangeControlState(context.Context) ([]byte, int64, error) {
	return append([]byte(nil), s.payload...), s.revision, nil
}

func (compatibilityStateStore) SaveChangeControlState(context.Context, int64, []byte) (int64, error) {
	return 0, nil
}
