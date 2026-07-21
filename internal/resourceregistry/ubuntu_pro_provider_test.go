package resourceregistry_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

type ubuntuProProviderRunner struct {
	statusOutputs [][]byte
	attachOutput  []byte
	runCalls      int
	inputCalls    int
}

func (r *ubuntuProProviderRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.runCalls++
	if name != "/usr/bin/pro" || len(args) != 2 || args[0] != "api" || args[1] != "u.pro.status.is_attached.v1" {
		return nil, nil, fmt.Errorf("unexpected read-only process %s %v", name, args)
	}
	if len(r.statusOutputs) == 0 {
		return nil, nil, fmt.Errorf("unexpected attachment status call")
	}
	output := r.statusOutputs[0]
	r.statusOutputs = r.statusOutputs[1:]
	return output, nil, nil
}

func (r *ubuntuProProviderRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	r.inputCalls++
	if name != "/usr/bin/pro" || len(args) != 4 || args[0] != "api" || args[1] != "u.pro.attach.token.full_token_attach.v1" || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected protected process %s %v", name, args)
	}
	return append([]byte(nil), r.attachOutput...), nil, nil
}

type ubuntuProProviderResolver struct {
	request  secrets.ResolveRequest
	calls    int
	material []byte
}

func (r *ubuntuProProviderResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.calls++
	r.request = request
	return secrets.Resolved{Provider: secrets.ProviderRemotr, Version: "7", ActivationGeneration: 3, Material: r.material}, nil
}

// OS-UPM-010, OS-UPM-011, OS-UPM-014, and OS-UPM-016: token resolution and
// protected attachment are reachable only after exact platform and attachment
// preflight, with a typed post-mutation observation.
func TestUbuntuProProviderPreflightPrecedesTokenResolutionAndAttachment(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: ubuntuPro
name: primary-subscription
lifecycle: attached
tokenRef: remotr:ubuntu-pro/production@active
`), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}

	exactUbuntu := facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", OSID: "ubuntu", OSReleaseSourceCount: 2,
		OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt,
	}
	statusEnvelope := func(attached bool) []byte {
		return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"is_attached":%t},"meta":{"environment_vars":[]},"type":"IsAttachedResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attached))
	}
	attachEnvelope := []byte(`{"_schema_version":"v1","data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)

	t.Run("unsupported derivative", func(t *testing.T) {
		runner := &ubuntuProProviderRunner{}
		resolver := &ubuntuProProviderResolver{material: []byte("unsupported-token-canary")}
		handler, err := resource.NewProvider(resourceregistry.FactoryContext{
			Facts:  facts.Facts{Distro: types.Debian, DistroVersion: "22.04", OSID: "pop", OSReleaseConsistent: true, Arch: types.X86, Package: types.Apt},
			Runner: runner, SecretResolver: resolver, ArtifactDigest: "sha256:artifact", ResourceAddress: "ubuntu-pro/primary-subscription",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := handler.Apply(context.Background()); err == nil {
			t.Fatal("unsupported provider Apply succeeded")
		}
		if runner.runCalls != 0 || runner.inputCalls != 0 || resolver.calls != 0 {
			t.Fatalf("unsupported preflight crossed external boundary: Run=%d RunInput=%d Resolve=%d", runner.runCalls, runner.inputCalls, resolver.calls)
		}
	})

	t.Run("already attached", func(t *testing.T) {
		runner := &ubuntuProProviderRunner{statusOutputs: [][]byte{statusEnvelope(true)}}
		resolver := &ubuntuProProviderResolver{material: []byte("already-attached-token-canary")}
		handler, err := resource.NewProvider(resourceregistry.FactoryContext{
			Facts: exactUbuntu, Runner: runner, SecretResolver: resolver, ArtifactDigest: "sha256:artifact", ResourceAddress: "ubuntu-pro/primary-subscription",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := handler.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		if runner.runCalls != 1 || runner.inputCalls != 0 || resolver.calls != 0 {
			t.Fatalf("already-attached path crossed mutation boundary: Run=%d RunInput=%d Resolve=%d", runner.runCalls, runner.inputCalls, resolver.calls)
		}
	})

	t.Run("eligible unattached", func(t *testing.T) {
		runner := &ubuntuProProviderRunner{statusOutputs: [][]byte{statusEnvelope(false), statusEnvelope(true)}, attachOutput: attachEnvelope}
		resolver := &ubuntuProProviderResolver{material: []byte("eligible-unattached-token-canary")}
		handler, err := resource.NewProvider(resourceregistry.FactoryContext{
			Facts: exactUbuntu, Runner: runner, SecretResolver: resolver, ArtifactDigest: "sha256:artifact", ResourceAddress: "ubuntu-pro/primary-subscription",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := handler.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		if runner.runCalls != 2 || runner.inputCalls != 1 || resolver.calls != 1 {
			t.Fatalf("unattached path calls: Run=%d RunInput=%d Resolve=%d", runner.runCalls, runner.inputCalls, resolver.calls)
		}
		wantRequest := secrets.ResolveRequest{
			Reference: "remotr:ubuntu-pro/production@active", ArtifactDigest: "sha256:artifact",
			ResourceAddress: "ubuntu-pro/primary-subscription", Purpose: "ubuntu-pro-token",
		}
		if resolver.request != wantRequest {
			t.Fatalf("Resolve() request = %#v, want %#v", resolver.request, wantRequest)
		}
		for index, value := range resolver.material {
			if value != 0 {
				t.Fatalf("resolved token byte %d was not cleared", index)
			}
		}
	})

}
