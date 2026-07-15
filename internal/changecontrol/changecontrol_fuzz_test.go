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
