package changecontrol

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func FuzzPersistedStateDecoderRoundTripsAcceptedBoundedInput(f *testing.F) {
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte(`{"version":1,"requests":{},"rollouts":{},"baselines":{},"policy":{},"automatic_promotion":{},"leases":{},"attempts":{},"break_glass":{}}`))
	f.Add([]byte(`{"version":1,"unknown":true}`))
	f.Add([]byte(`not-json`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 64*1024 {
			return
		}
		state, err := decodePersistedState(payload)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := decodePersistedState(encoded)
		if err != nil {
			t.Fatalf("accepted state did not decode after canonical encoding: %v", err)
		}
		if !reflect.DeepEqual(state, roundTripped) {
			t.Fatalf("canonical state changed across round trip: before=%+v after=%+v", state, roundTripped)
		}
	})
}

func FuzzAuthorizationGroupingPreservesEachResourceExactlyOnce(f *testing.F) {
	f.Add("guarded", "guarded")
	f.Add("network", "boot")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, firstGroup, secondGroup string) {
		if len(firstGroup)+len(secondGroup) > 128 || strings.ContainsAny(firstGroup+secondGroup, "\x00\n\r") {
			return
		}
		registry := NewRegistry(RegistryOptions{NewID: deterministicFuzzIDs("request")})
		requests, err := registry.CreateChangeRequests(FleetPlan{
			Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
			Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
			Resources: []ResourcePlan{
				{Address: "base/first", DesiredHash: "sha256:first", Risk: models.RiskConnectivity, Provider: "nftables", AuthorizationGroup: firstGroup},
				{Address: "base/second", DesiredHash: "sha256:second", Risk: models.RiskBoot, Provider: "systemd", AuthorizationGroup: secondGroup},
			},
		}, "operator")
		if err != nil {
			t.Fatal(err)
		}
		wantRequests := 2
		if firstGroup != "" && firstGroup == secondGroup {
			wantRequests = 1
		}
		if len(requests) != wantRequests {
			t.Fatalf("groups (%q, %q) produced %d requests, want %d", firstGroup, secondGroup, len(requests), wantRequests)
		}
		seen := map[string]int{}
		for _, request := range requests {
			for _, resource := range request.Resources {
				seen[resource.Address]++
			}
		}
		for _, address := range []string{"base/first", "base/second"} {
			if seen[address] != 1 {
				t.Fatalf("resource %q appears %d times across grouped requests", address, seen[address])
			}
		}
	})
}

func FuzzExecutionLeaseHonorsAttemptLimitAndExpiry(f *testing.F) {
	f.Add(uint8(1))
	f.Add(uint8(4))

	f.Fuzz(func(t *testing.T, rawLimit uint8) {
		attemptLimit := int(rawLimit%5) + 1
		now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		registry := NewRegistry(RegistryOptions{
			Now:   func() time.Time { return now },
			NewID: deterministicFuzzIDs("request", "rollout", "lease"),
		})
		requests, err := registry.CreateChangeRequests(FleetPlan{
			Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
			Targets:   []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
			Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "sha256:firewall", Risk: models.RiskConnectivity, Provider: "nftables"}},
		}, "operator")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{MaxConcurrency: 1, AttemptLimit: attemptLimit}, "operator", "CHG-FUZZ"); err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= attemptLimit; attempt++ {
			lease, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint", Ready: true})
			if err != nil || !issued {
				t.Fatalf("attempt %d was not issued: lease=%+v issued=%t err=%v", attempt, lease, issued, err)
			}
			if lease.Attempt != attempt || !lease.ExpiresAt.Equal(now.Add(5*time.Minute)) {
				t.Fatalf("attempt %d lease = %+v", attempt, lease)
			}
			if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint", Ready: true}); err != nil || issued {
				t.Fatalf("overlapping lease issued=%t err=%v", issued, err)
			}
			now = now.Add(5*time.Minute + time.Nanosecond)
		}
		if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint", Ready: true}); err != nil || issued {
			t.Fatalf("lease beyond attempt limit issued=%t err=%v", issued, err)
		}
	})
}

// FuzzPlanDependencyGraphIncludesExactNormalClosure proves that a bounded
// plan graph includes every transitive normal prerequisite exactly once.
// Unknown dependencies and dependencies crossing explicit high-risk
// authorization groups must reject the whole plan instead of producing a
// partial request.
func FuzzPlanDependencyGraphIncludesExactNormalClosure(f *testing.F) {
	f.Add(uint8(4), []byte{0b00000010}, uint8(0))
	f.Add(uint8(8), []byte{0xff, 0x55, 0xaa, 0x00}, uint8(0))
	f.Add(uint8(2), []byte{}, uint8(1))
	f.Add(uint8(2), []byte{}, uint8(2))

	f.Fuzz(func(t *testing.T, rawNodeCount uint8, edges []byte, rawMode uint8) {
		if len(edges) > 64 {
			return
		}
		mode := rawMode % 3
		nodeCount := int(rawNodeCount%8) + 1
		if mode == 2 && nodeCount < 2 {
			nodeCount = 2
		}
		resources := make([]ResourcePlan, nodeCount)
		for index := range resources {
			resources[index] = ResourcePlan{
				Address:     fmt.Sprintf("base/node-%d", index),
				DesiredHash: fmt.Sprintf("sha256:node-%d", index),
				Risk:        models.RiskNormal,
				Provider:    "fuzz-provider",
			}
		}
		resources[0].Risk = models.RiskConnectivity
		resources[0].AuthorizationGroup = "fuzz-root"
		for from := range resources {
			for to := range resources {
				if fuzzGraphBit(edges, from*nodeCount+to) {
					resources[from].DependsOn = append(resources[from].DependsOn, resources[to].Address)
				}
			}
		}
		switch mode {
		case 1:
			resources[0].DependsOn = append(resources[0].DependsOn, "base/missing")
		case 2:
			resources[1].Risk = models.RiskAccess
			resources[1].AuthorizationGroup = "fuzz-other"
			resources[0].DependsOn = append(resources[0].DependsOn, resources[1].Address)
		}

		registry := NewRegistry(RegistryOptions{
			Now:   func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) },
			NewID: func() string { return "fuzz-request" },
		})
		requests, err := registry.CreateChangeRequests(FleetPlan{
			Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
			Resources: resources,
		}, "fuzzer")
		if mode != 0 {
			if err == nil {
				t.Fatalf("invalid dependency mode %d produced requests: %+v", mode, requests)
			}
			if len(requests) != 0 || len(registry.List()) != 0 {
				t.Fatalf("rejected dependency graph left partial requests: returned=%+v stored=%+v", requests, registry.List())
			}
			return
		}
		if err != nil {
			t.Fatalf("bounded dependency graph was rejected: %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("dependency graph produced %d requests, want 1: %+v", len(requests), requests)
		}

		want := fuzzDependencyClosure(resources, resources[0].Address)
		request := requests[0]
		if request.AuthorizationGroup != "fuzz-root" || request.Risk != models.RiskConnectivity {
			t.Fatalf("request identity = group:%q risk:%q", request.AuthorizationGroup, request.Risk)
		}
		if len(request.Resources) != len(want) || len(request.ResourceHashes) != len(want) {
			t.Fatalf("closure size = resources:%d hashes:%d want:%d", len(request.Resources), len(request.ResourceHashes), len(want))
		}
		seen := make(map[string]struct{}, len(request.Resources))
		for _, resource := range request.Resources {
			if _, duplicate := seen[resource.Address]; duplicate {
				t.Fatalf("resource %q appears more than once in dependency closure", resource.Address)
			}
			seen[resource.Address] = struct{}{}
			if _, ok := want[resource.Address]; !ok {
				t.Fatalf("unrelated resource %q entered dependency closure %v", resource.Address, want)
			}
			if request.ResourceHashes[resource.Address] != resource.DesiredHash {
				t.Fatalf("resource %q hash was not frozen exactly", resource.Address)
			}
			for _, dependency := range resource.DependsOn {
				if _, ok := seen[dependency]; ok {
					continue
				}
				if _, ok := want[dependency]; !ok {
					t.Fatalf("resource %q dependency %q is outside expected closure", resource.Address, dependency)
				}
			}
		}
		for address := range want {
			if _, ok := seen[address]; !ok {
				t.Fatalf("transitive dependency %q was omitted from request", address)
			}
		}
	})
}

func fuzzGraphBit(input []byte, index int) bool {
	byteIndex := index / 8
	if byteIndex >= len(input) {
		return false
	}
	return input[byteIndex]&(1<<uint(index%8)) != 0
}

func fuzzDependencyClosure(resources []ResourcePlan, root string) map[string]struct{} {
	byAddress := make(map[string]ResourcePlan, len(resources))
	for _, resource := range resources {
		byAddress[resource.Address] = resource
	}
	closure := make(map[string]struct{}, len(resources))
	queue := []string{root}
	for len(queue) > 0 {
		address := queue[0]
		queue = queue[1:]
		if _, seen := closure[address]; seen {
			continue
		}
		closure[address] = struct{}{}
		queue = append(queue, byAddress[address].DependsOn...)
	}
	return closure
}

func deterministicFuzzIDs(prefixes ...string) func() string {
	next := 0
	return func() string {
		prefix := prefixes[len(prefixes)-1]
		if next < len(prefixes) {
			prefix = prefixes[next]
		}
		next++
		return fmt.Sprintf("%s-%d", prefix, next)
	}
}
