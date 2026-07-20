package hostname_test

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: an endpoint without the declared systemd-hostnamed boundary is
// unsupported rather than ordinary hostname drift or a generic probe failure.
func TestApplicator_reportsMissingHostnamectlAsUnsupported(t *testing.T) {
	static := "qualified.example.test"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"hostnamectl [--static]": {Err: exec.ErrNotFound},
	}}
	applicator := hostname.New(models.HostnameResource{Name: "qualified", Static: &static}, runner)
	check := applicator.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != "hostname_provider_unsupported" {
		t.Fatalf("missing hostnamectl Check = %+v, want unsupported provider", check)
	}
}

// OS-AEC-098: systemd-hostnamed gives a configured static hostname precedence
// over a distinct transient value. The public provider seam must report that
// unsupported combination before either hostname scope is mutated.
func TestProviderRejectsTransientHostnameShadowedByStaticWithoutMutation(t *testing.T) {
	static, transient := "static.example.test", "transient"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"hostnamectl [--static]":    {Stdout: []byte("old.example.test\n")},
		"hostnamectl [--transient]": {Stdout: []byte("old.example.test\n")},
	}}
	provider, err := contract.New(hostname.New(models.HostnameResource{
		Name: "shadowed", Static: &static, Transient: &transient,
	}, runner))
	if err != nil {
		t.Fatal(err)
	}

	check := provider.Check(context.Background())
	if check.Status != contract.Unsupported || check.ReasonCode != "hostname_transient_shadowed_by_static" {
		t.Fatalf("shadowed hostname Check = %+v, want unsupported provider constraint", check)
	}
	runner.Calls = nil
	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "static hostname takes precedence") {
		t.Fatalf("shadowed hostname Apply = %+v, want pre-mutation failure", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("shadowed hostname Apply mutated or probed state: %#v", runner.Calls)
	}
}

// OS-KHB-007: static and transient hostnames are managed through distinct
// hostnamectl operations; no /etc/hosts ownership is implied.
func TestApplicator_convergesStaticAndTransientHostnamesSeparately(t *testing.T) {
	static, transient := "build.example.test", "build"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"hostnamectl [--static]":                                 {Stdout: []byte("old.example.test\n")},
		"hostnamectl [--transient]":                              {Stdout: []byte("old\n")},
		"hostnamectl [set-hostname --static build.example.test]": {},
		"hostnamectl [set-hostname --transient build]":           {},
	}}
	applicator := hostname.New(models.HostnameResource{Name: "build", Static: &static, Transient: &transient}, runner)
	if _, compliant := applicator.State(context.Background()); compliant {
		t.Fatal("State() unexpectedly compliant")
	}
	runner.Calls = nil
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := [][]string{{"--static"}, {"set-hostname", "--static", static}, {"--transient"}, {"set-hostname", "--transient", transient}}
	if len(runner.Calls) != len(want) {
		t.Fatalf("hostnamectl calls = %#v", runner.Calls)
	}
	for i, call := range runner.Calls {
		if call.Name != "hostnamectl" || !slices.Equal(call.Args, want[i]) {
			t.Fatalf("call %d = %#v, want hostnamectl %#v", i, call, want[i])
		}
	}
}
