package executor

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkActivationPlan []ActivationSignal

func BenchmarkCollectActivations(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		results := make([]ApplyResult, size)
		for i := range results {
			target := fmt.Sprintf("service-%04d", i)
			results[i] = ApplyResult{
				Status: Changed,
				Activation: []ActivationSignal{
					{Kind: ActivationRestart, Target: target},
					{Kind: ActivationDaemonReload},
					{Kind: ActivationRestart, Target: target},
				},
			}
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkActivationPlan = CollectActivations(results)
			}
		})
	}
}
