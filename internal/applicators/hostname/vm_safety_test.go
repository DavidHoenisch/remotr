//go:build vmsafety

package hostname_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs as root in the disposable Ubuntu system-safety VM. It
// proves real static/transient independence, immediate activation outcome,
// /etc/hosts preservation, idempotent Apply, and compliant second Checks.
func TestHostnameProviderContractVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("hostname VM contract must run as root")
	}
	originalStatic := vmReadHostname(t, "--static")
	originalTransient := vmReadHostname(t, "--transient")
	originalHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("hostnamectl", "set-hostname", "--static", originalStatic).Run()
		_ = exec.Command("hostnamectl", "set-hostname", "--transient", originalTransient).Run()
	})

	static := "remotr-vm-qualified"
	transient := static
	provider := vmHostnameProvider(t, models.HostnameResource{
		Name: "vm-qualified", Static: &static, Transient: &transient,
	})
	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("hostname drift Check = %+v, want drifted", check)
	}
	result := provider.Apply(context.Background())
	if result.Status != contract.Changed || result.Err != nil || len(result.Activation) != 0 || result.RebootRequired != contract.RebootNotRequired {
		t.Fatalf("hostname Apply = %+v, want immediate changed outcome", result)
	}
	if got := vmReadHostname(t, "--static"); got != static {
		t.Fatalf("static hostname = %q, want %q", got, static)
	}
	if got := vmReadHostname(t, "--transient"); got != transient {
		t.Fatalf("transient hostname = %q, want %q", got, transient)
	}
	vmAssertHostnameSecondCheck(t, provider)

	shadowed := "remotr-vm-shadowed"
	shadowedProvider := vmHostnameProvider(t, models.HostnameResource{
		Name: "vm-shadowed", Transient: &shadowed,
	})
	if check := shadowedProvider.Check(context.Background()); check.Status != contract.Unsupported || check.ReasonCode != "hostname_transient_shadowed_by_static" {
		t.Fatalf("shadowed transient Check = %+v, want unsupported static precedence", check)
	}
	if result := shadowedProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("shadowed transient Apply = %+v, want pre-mutation failure", result)
	}
	if got := vmReadHostname(t, "--static"); got != static {
		t.Fatalf("shadowed transient Apply changed static hostname to %q", got)
	}
	if got := vmReadHostname(t, "--transient"); got != transient {
		t.Fatalf("shadowed transient Apply changed active hostname to %q", got)
	}

	vmSetHostname(t, "--static", "")
	vmSetHostname(t, "--transient", "remotr-vm-transient-base")
	transientOnly := "remotr-vm-transient-two"
	transientProvider := vmHostnameProvider(t, models.HostnameResource{
		Name: "vm-transient-only", Transient: &transientOnly,
	})
	if result := transientProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("transient-only Apply = %+v, want changed", result)
	}
	if got := vmReadHostname(t, "--static"); got != "" {
		t.Fatalf("transient-only Apply created static hostname %q", got)
	}
	if got := vmReadHostname(t, "--transient"); got != transientOnly {
		t.Fatalf("transient-only Apply = %q, want %q", got, transientOnly)
	}
	vmAssertHostnameSecondCheck(t, transientProvider)

	staticOnly := "remotr-vm-static-two"
	staticProvider := vmHostnameProvider(t, models.HostnameResource{
		Name: "vm-static-only", Static: &staticOnly,
	})
	if result := staticProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("static-only Apply = %+v, want changed", result)
	}
	if got := vmReadHostname(t, "--static"); got != staticOnly {
		t.Fatalf("static-only Apply = %q, want %q", got, staticOnly)
	}
	if got := vmReadHostname(t, "--transient"); got != staticOnly {
		t.Fatalf("static hostname was not activated immediately: got %q, want %q", got, staticOnly)
	}
	vmAssertHostnameSecondCheck(t, staticProvider)

	hosts, err := os.ReadFile("/etc/hosts")
	if err != nil || !bytes.Equal(hosts, originalHosts) {
		t.Fatalf("/etc/hosts changed during hostname management: err=%v", err)
	}
}

func vmHostnameProvider(t *testing.T, resource models.HostnameResource) contract.Provider {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(hostname.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmReadHostname(t *testing.T, flag string) string {
	t.Helper()
	output, err := exec.Command("hostnamectl", flag).CombinedOutput()
	if err != nil {
		t.Fatalf("hostnamectl %s: %v: %s", flag, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmSetHostname(t *testing.T, flag, value string) {
	t.Helper()
	output, err := exec.Command("hostnamectl", "set-hostname", flag, value).CombinedOutput()
	if err != nil {
		t.Fatalf("hostnamectl set-hostname %s: %v: %s", flag, err, output)
	}
}

func vmAssertHostnameSecondCheck(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("hostname second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant hostname Apply = %+v, want no change", result)
	}
}
