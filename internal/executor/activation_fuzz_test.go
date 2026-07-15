package executor

import (
	"slices"
	"testing"
)

func FuzzCollectActivationsIsOrderedAndDeduplicated(f *testing.F) {
	f.Add([]byte{4, 0, 2, 4, 8})
	f.Add([]byte{8, 7, 6, 5, 4, 3, 2, 1, 0})

	types := []ActivationSignal{
		{Kind: ActivationDaemonReload},
		{Kind: ActivationTrustStoreRefresh, Target: "debian"},
		{Kind: ActivationReload, Target: "auditd.service"},
		{Kind: ActivationTryRestart, Target: "collector.service"},
		{Kind: ActivationRestart, Target: "remotr-agent.service"},
		{Kind: ActivationLogoutRequired},
		{Kind: ActivationApplicationRestart, Target: "firefox"},
		{Kind: ActivationNextBoot},
		{Kind: ActivationRebootRequired},
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 256 {
			return
		}
		results := make([]ApplyResult, 0, len(input))
		buckets := make([][]ActivationSignal, len(types))
		seen := map[string]bool{}
		for _, value := range input {
			rank := int((value & 0x0f) % byte(len(types)))
			signal := types[rank]
			results = append(results, ApplyResult{Activation: []ActivationSignal{signal}})
			key := string(signal.Kind) + "\x00" + signal.Target
			if !seen[key] {
				seen[key] = true
				buckets[rank] = append(buckets[rank], signal)
			}
		}
		want := make([]ActivationSignal, 0, len(seen))
		for _, bucket := range buckets {
			want = append(want, bucket...)
		}
		if got := CollectActivations(results); !slices.Equal(got, want) {
			t.Fatalf("CollectActivations(%v) = %v, want %v", input, got, want)
		}
	})
}
