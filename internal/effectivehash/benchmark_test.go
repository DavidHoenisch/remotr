package effectivehash_test

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkEffectiveHash string

func BenchmarkCanonicalEffectiveHash(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		inputs := make([]effectivehash.Input, int(size))
		for index := range inputs {
			inputs[index] = effectivehash.Input{
				ResourceAddress: fmt.Sprintf("benchmark/resource-%04d", index),
				ResourceKind:    "service",
				Provider: effectivehash.ProviderIdentity{
					ID: "systemd", ContractRevision: "service-v1",
				},
				Desired: effectivehash.Object{
					"name":    effectivehash.String(fmt.Sprintf("service-%04d", index)),
					"enabled": effectivehash.Boolean(index%2 == 0),
					"ports": effectivehash.Set{
						effectivehash.Integer(22), effectivehash.Integer(443),
					},
					"labels": effectivehash.Object{
						"environment": effectivehash.String("benchmark"),
						"ordinal":     effectivehash.Integer(index),
					},
				},
				Defaults: effectivehash.Object{
					"restart": effectivehash.Boolean(false),
				},
				Secrets: []effectivehash.SecretIdentity{{
					Path: "credentialRef", Provider: "remotr",
					Name: fmt.Sprintf("service/%04d", index), Version: "0001",
					ActivationGeneration: uint64(index), Purpose: "service-credential",
				}},
			}
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				for _, input := range inputs {
					digest, err := effectivehash.Sum(input)
					if err != nil {
						b.Fatal(err)
					}
					benchmarkEffectiveHash = digest
				}
			}
		})
	}
}
