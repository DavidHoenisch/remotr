package secrets

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

var benchmarkResolvedGlobalSecret Resolved

func BenchmarkGlobalSecretActivationPlanning(b *testing.B) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	service := benchmarkRegistryService(b, completeActivationPlanner(hash, ""), nil)
	if _, err := service.Upload(b.Context(), UploadRequest{Name: "benchmark/global", Scope: ScopeGlobal, Material: []byte("benchmark-canary"), ActorID: "benchmark"}); err != nil {
		b.Fatal(err)
	}
	uses := make([]ActivationUse, 0, 1000)
	for fleet := range 100 {
		for resource := range 10 {
			uses = append(uses, ActivationUse{
				Fleet: fmt.Sprintf("fleet-%03d", fleet), ResourceAddress: fmt.Sprintf("config/resource-%03d-%02d", fleet, resource),
				Purpose: "repository-credential", Risk: models.RiskNormal,
			})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := service.Activate(b.Context(), ActivationRequest{Name: "benchmark/global", Version: "1", ActorID: "benchmark", Uses: uses}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGlobalSecretAuthenticatedResolution(b *testing.B) {
	const hash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	service := benchmarkRegistryService(b, completeActivationPlanner(hash, ""), nil)
	if _, err := service.Upload(b.Context(), UploadRequest{Name: "benchmark/global", Scope: ScopeGlobal, Material: []byte("benchmark-canary"), ActorID: "benchmark"}); err != nil {
		b.Fatal(err)
	}
	use := ActivationUse{Fleet: "fleet-001", ResourceAddress: "config/resource", Purpose: "repository-credential", Risk: models.RiskNormal}
	if _, err := service.Activate(b.Context(), ActivationRequest{Name: "benchmark/global", Version: "1", ActorID: "benchmark", Uses: []ActivationUse{use}}); err != nil {
		b.Fatal(err)
	}
	request := ResolveRequest{Reference: "remotr:benchmark/global@active", Fleet: use.Fleet, EndpointID: "endpoint-1", ResourceAddress: use.ResourceAddress, Purpose: use.Purpose}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		resolved, err := service.Resolve(b.Context(), request)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkResolvedGlobalSecret = resolved
		clear(resolved.Material)
	}
}

func benchmarkRegistryService(b *testing.B, planner ActivationPlanner, gate RolloutGate) *RegistryService {
	b.Helper()
	keyring, err := NewKeyring("benchmark-kek", map[string][]byte{"benchmark-kek": bytes.Repeat([]byte{0xd1}, 32)})
	if err != nil {
		b.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		b.Fatal(err)
	}
	service, err := NewRegistryService(NewMemoryVersionRepository(), envelope, planner, gate)
	if err != nil {
		b.Fatal(err)
	}
	return service
}
