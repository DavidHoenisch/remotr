package hostlocale_test

import (
	"context"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-008: a timezone-only resource must not query or change locale or
// keymap state.
func TestApplicator_ApplyTimezoneWithoutTouchingLocaleOrKeymap(t *testing.T) {
	timezone := "Europe/Berlin"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"timedatectl [show --property=Timezone --value]": {Stdout: []byte("UTC\n")},
		"timedatectl [set-timezone Europe/Berlin]":       {},
	}}
	applicator := hostlocale.New(models.HostLocaleResource{Name: "berlin", Timezone: &timezone}, runner)

	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := [][]string{{"show", "--property=Timezone", "--value"}, {"set-timezone", timezone}}
	if len(runner.Calls) != len(want) {
		t.Fatalf("calls = %#v, want timedatectl %#v", runner.Calls, want)
	}
	for i, call := range runner.Calls {
		if call.Name != "timedatectl" || !slices.Equal(call.Args, want[i]) {
			t.Fatalf("call %d = %#v, want timedatectl %#v", i, call, want[i])
		}
	}
}

// OS-KHB-010: applying locale changes records a new-login requirement but
// never terminates a user session as an incidental provider action.
func TestApplicator_ApplyResultReportsLogoutForLocaleChange(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"localectl [status --no-pager]":           {Stdout: []byte("System Locale: LANG=C\n")},
		"localectl [set-locale LANG=de_DE.UTF-8]": {},
	}}
	applicator := hostlocale.New(models.HostLocaleResource{Name: "german", Locale: map[string]string{"LANG": "de_DE.UTF-8"}}, runner)

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed || !slices.Equal(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationLogoutRequired}}) || result.RebootRequired != executor.RebootNotRequired {
		t.Fatalf("ApplyResult() = %+v, want changed/logout-required without reboot", result)
	}
	for _, call := range runner.Calls {
		if call.Name != "localectl" {
			t.Fatalf("unexpected session-changing command %#v", call)
		}
	}
}

func TestApplicator_ApplyResultReportsRebootForConsoleKeymapChange(t *testing.T) {
	keymap := "de"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"localectl [status --no-pager]": {Stdout: []byte("    VC Keymap: us\n")},
		"localectl [set-keymap de]":     {},
	}}
	applicator := hostlocale.New(models.HostLocaleResource{Name: "german-console", Keymap: &keymap}, runner)

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed || !slices.Equal(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}) || result.RebootRequired != executor.RebootRequired {
		t.Fatalf("ApplyResult() = %+v, want changed/reboot-required", result)
	}
}
