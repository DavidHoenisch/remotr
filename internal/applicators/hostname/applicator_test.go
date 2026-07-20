package hostname_test

import (
	"context"
	"os/exec"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
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
