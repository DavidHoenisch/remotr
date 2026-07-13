package hostname_test

import (
	"context"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

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
