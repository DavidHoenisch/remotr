//go:build vmsafety

package ubuntupro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

const ubuntuProSyntheticCanary = "remotr-synthetic-ubuntu-pro-token-canary"

type ubuntuProVMAPIDouble struct {
	attached  bool
	readCalls []string
	input     []byte
}

func (runner *ubuntuProVMAPIDouble) Run(name string, args ...string) ([]byte, []byte, error) {
	return runner.RunContext(context.Background(), name, args...)
}

func (runner *ubuntuProVMAPIDouble) RunContext(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if name != proExecutable || len(args) != 2 || args[0] != "api" {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro read boundary: %s %v", name, args)
	}
	runner.readCalls = append(runner.readCalls, args[1])
	switch args[1] {
	case isAttachedEndpoint:
		return ubuntuProVMAttachmentEnvelope(runner.attached), nil, nil
	case detachEndpoint:
		runner.attached = false
		return ubuntuProVMDetachEnvelope(), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro API endpoint: %s", args[1])
	}
}

func (runner *ubuntuProVMAPIDouble) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.RunInputContext(context.Background(), name, input, args...)
}

func (runner *ubuntuProVMAPIDouble) RunInputContext(ctx context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	want := []string{"api", fullTokenAttachEndpoint, "--data", "-"}
	if name != proExecutable || !slices.Equal(args, want) {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro mutation boundary: %s %v", name, args)
	}
	var request struct {
		Token              string `json:"token"`
		AutoEnableServices bool   `json:"auto_enable_services"`
	}
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, nil, fmt.Errorf("decode protected attach request: %w", err)
	}
	if request.Token != ubuntuProSyntheticCanary || request.AutoEnableServices {
		return nil, nil, fmt.Errorf("unexpected protected attach request")
	}
	runner.input = append([]byte(nil), input...)
	runner.attached = true
	return ubuntuProVMAttachEnvelope(), nil, nil
}

func ubuntuProVMAttachmentEnvelope(attached bool) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"is_attached":%t},"meta":{"environment_vars":[]},"type":"IsAttachedResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attached))
}

func ubuntuProVMAttachEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

func ubuntuProVMDetachEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"disabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"DetachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

func ubuntuProVMFacts(t *testing.T) facts.Facts {
	t.Helper()
	release := os.Getenv("REMOTR_UBUNTU_PRO_VM_RELEASE")
	if release == "" {
		t.Fatal("REMOTR_UBUNTU_PRO_VM_RELEASE is required")
	}
	endpoint, err := facts.Read()
	if err != nil {
		t.Fatalf("read guest facts: %v", err)
	}
	if !endpoint.ExactUbuntu() || endpoint.DistroVersion != release || endpoint.Arch != types.X86 || endpoint.Package != types.Apt {
		t.Fatalf("guest facts = %+v, want exact Ubuntu %s amd64 with APT", endpoint, release)
	}
	return endpoint
}

func ubuntuProVMResolver(t *testing.T, calls *int) TokenResolver {
	t.Helper()
	path := os.Getenv("REMOTR_UBUNTU_PRO_TOKEN_FILE")
	return func(ctx context.Context, reference string) ([]byte, error) {
		*calls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if reference != "remotr:ubuntu-pro/vm@synthetic" {
			return nil, fmt.Errorf("unexpected token reference")
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat synthetic token: %w", err)
		}
		if info.Mode().Perm() != 0o600 {
			return nil, fmt.Errorf("synthetic token mode = %o, want 600", info.Mode().Perm())
		}
		material, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read synthetic token: %w", err)
		}
		return bytes.TrimSpace(material), nil
	}
}

// OS-UPM-010, OS-UPM-011, OS-UPM-014, OS-UPM-024, OS-UPM-025, and
// OS-UPM-027: a real pinned Ubuntu guest exercises the public provider while a
// deterministic API double prevents any Canonical subscription consumption.
func TestUbuntuProProviderContractVM(t *testing.T) {
	endpoint := ubuntuProVMFacts(t)
	runner := &ubuntuProVMAPIDouble{}
	resolverCalls := 0
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "vm-subscription", TokenRef: "remotr:ubuntu-pro/vm@synthetic",
	}
	applicator := New(resource, endpoint, runner, ubuntuProVMResolver(t, &resolverCalls))
	first := executor.New().Apply(context.Background(), applicator)
	if first.Status != executor.Changed || resolverCalls != 1 || !runner.attached {
		t.Fatalf("first Apply() = %+v, resolver calls = %d, attached = %t", first, resolverCalls, runner.attached)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("Check() = %+v", check)
	}
	second := executor.New().Apply(context.Background(), applicator)
	if second.Status != executor.NoChange || resolverCalls != 1 {
		t.Fatalf("second Apply() = %+v, resolver calls = %d", second, resolverCalls)
	}
	detached := resource
	detached.Lifecycle = models.UbuntuProDetached
	detached.TokenRef = ""
	detach := executor.New().Apply(context.Background(), New(detached, endpoint, runner, nil))
	if detach.Status != executor.Changed || runner.attached {
		t.Fatalf("detach Apply() = %+v, attached = %t", detach, runner.attached)
	}
	if !bytes.Contains(runner.input, []byte(ubuntuProSyntheticCanary)) {
		t.Fatal("protected stdin did not carry the synthetic canary")
	}
	for _, result := range []any{first, check, second, detach} {
		if strings.Contains(fmt.Sprintf("%+v", result), ubuntuProSyntheticCanary) {
			t.Fatalf("public result exposed synthetic token: %+v", result)
		}
	}
}

// OS-UPM-003 and OS-UPM-004: Ubuntu-derived, ambiguous, interim, and future
// identities fail before API or secret boundaries, even inside an Ubuntu VM.
func TestUbuntuProNegativeIdentitiesVM(t *testing.T) {
	ubuntuProVMFacts(t)
	tests := []struct {
		name  string
		facts facts.Facts
	}{
		{name: "pop-os", facts: facts.Facts{Distro: types.Debian, DistroVersion: "22.04", OSID: "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "System76", Arch: types.X86, Package: types.Apt}},
		{name: "linux-mint", facts: facts.Facts{Distro: types.Debian, DistroVersion: "22", OSID: "linuxmint", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Linux Mint", Arch: types.X86, Package: types.Apt}},
		{name: "conflicting-os-release", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: false, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
		{name: "interim-ubuntu", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "25.10", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
		{name: "future-ubuntu", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "28.04", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &ubuntuProVMAPIDouble{}
			resolverCalls := 0
			result := executor.New().Apply(context.Background(), New(models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "negative-identity", TokenRef: "remotr:ubuntu-pro/vm@synthetic",
			}, test.facts, runner, ubuntuProVMResolver(t, &resolverCalls)))
			if result.Status != executor.Failed || result.Err == nil || len(runner.readCalls) != 0 || len(runner.input) != 0 || resolverCalls != 0 {
				t.Fatalf("Apply() = %+v, reads = %v, input = %q, resolver calls = %d", result, runner.readCalls, runner.input, resolverCalls)
			}
		})
	}
}
