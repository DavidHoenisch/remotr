package server

import "testing"

func TestFastPathMutationScopeRegistryIsComplete(t *testing.T) {
	tests := map[fastPathMutationClass]cacheScope{
		mutationRelease: cacheScopeGlobal, mutationSchedule: cacheScopeGlobal,
		mutationFleetPolicy: cacheScopeFleet, mutationFleetUpgrade: cacheScopeFleet,
		mutationEndpointEnrollment: cacheScopeEndpoint, mutationEndpointDelete: cacheScopeEndpoint,
		mutationEndpointReassign: cacheScopeEndpoint, mutationEndpointLabels: cacheScopeEndpoint,
		mutationEndpointUpgrade: cacheScopeEndpoint, mutationDiagnostics: cacheScopeEndpoint,
		mutationChangeControl: cacheScopeGlobal, mutationSecretLifecycle: cacheScopeGlobal,
		mutationEnrollmentToken: cacheScopeGlobal, mutationDeploymentToken: cacheScopeGlobal,
	}
	for class, want := range tests {
		if got, ok := fastPathMutationScopes[class]; !ok || got != want {
			t.Errorf("mutation %q scope = %v, %t; want %v", class, got, ok, want)
		}
		t.Run(string(class), func(t *testing.T) {
			srv := New(Config{FastPath: FastPathConfig{Enabled: true, ServingProcesses: 1}})
			primeEndpointDecision(srv.fastPath, "target", "engineering")
			primeEndpointDecision(srv.fastPath, "same-fleet", "engineering")
			primeEndpointDecision(srv.fastPath, "unrelated", "finance")
			key := "target"
			if want == cacheScopeFleet {
				key = "engineering"
			}
			complete := srv.beginFastPathMutation(class, key)
			defer complete()
			if _, present := srv.fastPath.entries["target"]; present {
				t.Fatal("target entry survived mutation begin")
			}
			_, sameFleetPresent := srv.fastPath.entries["same-fleet"]
			_, unrelatedPresent := srv.fastPath.entries["unrelated"]
			if want == cacheScopeGlobal && (sameFleetPresent || unrelatedPresent) {
				t.Fatal("global mutation retained cached entries")
			}
			if want == cacheScopeFleet && (sameFleetPresent || !unrelatedPresent) {
				t.Fatal("fleet mutation did not isolate eviction to its fleet")
			}
			if want == cacheScopeEndpoint && (!sameFleetPresent || !unrelatedPresent) {
				t.Fatal("endpoint mutation evicted unrelated decisions")
			}
		})
	}
}
