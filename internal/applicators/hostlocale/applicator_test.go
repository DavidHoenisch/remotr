package hostlocale_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
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

// OS-AEC-098: a combined host-locale resource is one public provider action;
// if native locale application fails after timezone mutation, the previously
// observed timezone must be restored before Apply reports failure.
func TestProviderRestoresTimezoneWhenLocaleApplyFails(t *testing.T) {
	timezone := "Europe/Berlin"
	runner := &hostLocaleFailureRunner{timezone: "UTC", locale: "C", failLocale: errors.New("native locale rejected")}
	provider, err := contract.New(hostlocale.New(models.HostLocaleResource{
		Name: "transactional", Timezone: &timezone, Locale: map[string]string{"LANG": "de_DE.UTF-8"},
	}, runner))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("failed combined host-locale Apply = %+v, want failed", result)
	}
	if runner.timezone != "UTC" {
		t.Fatalf("timezone after failed locale Apply = %q, want restored UTC", runner.timezone)
	}
	want := [][]string{
		{"show", "--property=Timezone", "--value"},
		{"set-timezone", "Europe/Berlin"},
		{"status", "--no-pager"},
		{"set-locale", "LANG=de_DE.UTF-8"},
		{"set-timezone", "UTC"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("native boundary calls = %#v, want %#v", runner.calls, want)
	}
	for index := range want {
		if !slices.Equal(runner.calls[index], want[index]) {
			t.Fatalf("native boundary call %d = %#v, want %#v", index, runner.calls[index], want[index])
		}
	}
}

func TestProviderRestoresLocaleWhenKeymapApplyFails(t *testing.T) {
	keymap := "de"
	runner := &hostLocaleFailureRunner{
		locale: "C", keymap: "us", failKeymap: errors.New("native keymap rejected"),
	}
	provider, err := contract.New(hostlocale.New(models.HostLocaleResource{
		Name: "transactional", Locale: map[string]string{"LANG": "de_DE.UTF-8"}, Keymap: &keymap,
	}, runner))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("failed locale/keymap Apply = %+v, want failed", result)
	}
	if runner.locale != "C" {
		t.Fatalf("locale after failed keymap Apply = %q, want restored C", runner.locale)
	}
	want := [][]string{
		{"status", "--no-pager"},
		{"set-locale", "LANG=de_DE.UTF-8"},
		{"set-keymap", "de"},
		{"set-locale", "LANG=C"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("native boundary calls = %#v, want %#v", runner.calls, want)
	}
	for index := range want {
		if !slices.Equal(runner.calls[index], want[index]) {
			t.Fatalf("native boundary call %d = %#v, want %#v", index, runner.calls[index], want[index])
		}
	}
}

type hostLocaleFailureRunner struct {
	timezone   string
	locale     string
	keymap     string
	failLocale error
	failKeymap error
	calls      [][]string
}

func (r *hostLocaleFailureRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	switch {
	case name == "timedatectl" && slices.Equal(args, []string{"show", "--property=Timezone", "--value"}):
		return []byte(r.timezone + "\n"), nil, nil
	case name == "timedatectl" && len(args) == 2 && args[0] == "set-timezone":
		r.timezone = args[1]
		return nil, nil, nil
	case name == "localectl" && slices.Equal(args, []string{"status", "--no-pager"}):
		return []byte(fmt.Sprintf("System Locale: LANG=%s\n    VC Keymap: %s\n", r.locale, r.keymap)), nil, nil
	case name == "localectl" && len(args) == 2 && args[0] == "set-locale":
		if r.failLocale != nil {
			return nil, []byte("invalid locale"), r.failLocale
		}
		r.locale = strings.TrimPrefix(args[1], "LANG=")
		return nil, nil, nil
	case name == "localectl" && len(args) == 2 && args[0] == "set-keymap":
		if r.failKeymap != nil {
			return nil, []byte("invalid keymap"), r.failKeymap
		}
		r.keymap = args[1]
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
}
