//go:build vmsafety

package trustanchors_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/trustanchors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// TestTrustAnchorProviderVM exercises the registered Ubuntu provider against
// the native trust directories and update-ca-certificates activation boundary.
func TestTrustAnchorProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM trust-anchor provider test must run as root")
	}
	vmAssertTrustAnchorUbuntu2404(t)

	ctx := context.Background()
	stateDir := t.TempDir()
	resource := models.TrustAnchorResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-qualified", AnchorRef: "remotr:trust-anchors/vm-qualified@active",
	}
	material, fingerprint := testAnchor(t, "Remotr VM Qualified Root")
	resource.Fingerprint = fingerprint
	managedPath := filepath.Join("/usr/local/share/ca-certificates", "remotr-vm-qualified.crt")
	unmanagedPath := filepath.Join("/usr/local/share/ca-certificates", "remotr-vm-unmanaged.keep")
	if err := os.Remove(managedPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanagedPath, []byte("preserve\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(managedPath)
		_ = os.Remove(unmanagedPath)
		_ = exec.Command("update-ca-certificates").Run()
	})

	mismatch := resource
	mismatch.Fingerprint = "sha256:" + strings.Repeat("0", 64)
	mismatchProvider := vmRegisteredTrustAnchorProvider(t, mismatch, stateDir, "m5-security/mismatched-trust-anchor", &vmTrustAnchorResolver{material: material})
	if result := mismatchProvider.ApplyResult(ctx); result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "fingerprint mismatch") {
		t.Fatalf("mismatched trust-anchor ApplyResult = %+v, want pre-activation failure", result)
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("mismatched trust anchor mutated managed path: %v", err)
	}

	resolver := &vmTrustAnchorResolver{material: material}
	provider := vmRegisteredTrustAnchorProvider(t, resource, stateDir, "m5-security/managed-trust-anchor", resolver)
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial trust-anchor Check = %+v, want drifted", check)
	}
	result := provider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("trust-anchor ApplyResult = %+v, want changed transactional", result)
	}
	vmRunTrustRefresh(t, result, result)
	if len(resolver.requests) != 1 || resolver.requests[0].Purpose != "ca-trust-anchor" {
		t.Fatalf("trust-anchor secret resolution requests = %+v", resolver.requests)
	}
	if check := provider.Check(ctx); check.Status != executor.Compliant || !strings.Contains(string(check.ObservedSummary), fingerprint) {
		t.Fatalf("trust-anchor second Check = %+v, want compliant fingerprint", check)
	}
	vmVerifyTrustedAnchor(t, managedPath)

	restarted := vmRegisteredTrustAnchorProvider(t, resource, stateDir, "m5-security/managed-trust-anchor", &vmTrustAnchorResolver{material: material})
	if err := restarted.Revert(ctx); err != nil {
		t.Fatalf("reconstructed trust-anchor rollback: %v", err)
	}
	if check := restarted.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("Check after reconstructed rollback = %+v, want drifted", check)
	}
	if result := restarted.ApplyResult(ctx); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("trust-anchor reapply = %+v, want changed transactional", result)
	} else {
		vmRunTrustRefresh(t, result)
	}
	if check := restarted.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("trust-anchor second Check after reapply = %+v, want compliant", check)
	}

	absent := resource
	absent.Lifecycle, absent.AnchorRef, absent.Fingerprint = models.LifecycleAbsent, "", ""
	absentProvider := vmRegisteredTrustAnchorProvider(t, absent, stateDir, "m5-security/remove-trust-anchor", nil)
	result = absentProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("trust-anchor removal ApplyResult = %+v, want changed transactional", result)
	}
	vmRunTrustRefresh(t, result)
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("trust-anchor removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant trust-anchor removal ApplyResult = %+v, want no change", result)
	}
	if got, err := os.ReadFile(unmanagedPath); err != nil || string(got) != "preserve\n" {
		t.Fatalf("trust-anchor lifecycle changed unmanaged file: %q, %v", got, err)
	}
}

type vmTrustAnchorResolver struct {
	material []byte
	requests []secrets.ResolveRequest
}

func (r *vmTrustAnchorResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.requests = append(r.requests, request)
	if request.Purpose != "ca-trust-anchor" {
		return secrets.Resolved{}, fmt.Errorf("unexpected trust-anchor purpose %q", request.Purpose)
	}
	return secrets.Resolved{Provider: secrets.ProviderRemotr, Material: append([]byte(nil), r.material...)}, nil
}

func vmRegisteredTrustAnchorProvider(t *testing.T, resource models.TrustAnchorResource, stateDir, address string, resolver secrets.Resolver) *trustanchors.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{TrustAnchors: []models.TrustAnchorResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindTrustAnchor {
		t.Fatalf("trust-anchor registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04"}, StateDir: stateDir,
		SecretResolver: resolver, ArtifactDigest: "sha256:vm-trust-anchor", ResourceAddress: address,
	})
	provider, ok := handler.(*trustanchors.Applicator)
	if err != nil || !ok {
		t.Fatalf("trust-anchor registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmRunTrustRefresh(t *testing.T, results ...executor.ApplyResult) {
	t.Helper()
	want := []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: "debian"}}
	if got := executor.CollectActivations(results); !slices.Equal(got, want) {
		t.Fatalf("coalesced trust refresh = %+v, want %+v", got, want)
	}
	if output, err := exec.Command("update-ca-certificates").CombinedOutput(); err != nil {
		t.Fatalf("update-ca-certificates: %v: %s", err, output)
	}
}

func vmVerifyTrustedAnchor(t *testing.T, path string) {
	t.Helper()
	if output, err := exec.Command("openssl", "verify", "-CApath", "/etc/ssl/certs", path).CombinedOutput(); err != nil {
		t.Fatalf("native trust verification: %v: %s", err, output)
	}
}

func vmAssertTrustAnchorUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("trust-anchor VM OS release = %q, %v", raw, err)
	}
}
