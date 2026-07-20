//go:build vmsafety

package hostlocale_test

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098, OS-KHB-008, OS-KHB-010: run Ubuntu's real host-localization
// boundaries in the pinned disposable VM. Each optional scope must preserve
// omitted state, native rejection must restore earlier mutations, and
// successful changes must report truthful activation and second Checks.
func TestHostLocaleNativeKeymapValidationVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("host-locale VM contract must run as root")
	}
	localeBefore, keymapBefore := vmLocalectlState(t)
	timezoneBefore := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value")
	keymap := "remotr-invalid-keymap"
	provider := vmHostLocaleProvider(t, models.HostLocaleResource{Name: "native-validation", Keymap: &keymap})
	if check := provider.Check(context.Background()); check.Status != contract.Unsupported || check.ReasonCode != "host_locale_keymap_unsupported" {
		t.Fatalf("invalid Ubuntu keymap Check = %+v, want unsupported", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("invalid Ubuntu keymap Apply = %+v, want pre-mutation failure", result)
	}
	localeAfter, keymapAfter := vmLocalectlState(t)
	if !maps.Equal(localeAfter, localeBefore) || keymapAfter != keymapBefore {
		t.Fatalf("invalid keymap Apply changed locale/keymap: locale=%v keymap=%q", localeAfter, keymapAfter)
	}
	if got := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value"); got != timezoneBefore {
		t.Fatalf("invalid keymap Apply changed timezone to %q", got)
	}
}

func TestHostLocaleProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("host-locale VM contract must run as root")
	}
	ctx := context.Background()
	originalTimezone := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value")
	originalLocale, originalKeymap := vmLocalectlState(t)
	originalKeyboard, err := os.ReadFile("/etc/default/keyboard")
	if err != nil {
		t.Fatal(err)
	}
	keyboardInfo, err := os.Stat("/etc/default/keyboard")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("timedatectl", "set-timezone", originalTimezone).Run()
		args := append([]string{"set-locale"}, vmLocaleAssignments(originalLocale)...)
		if len(args) > 1 {
			_ = exec.Command("localectl", args...).Run()
		}
		_ = os.WriteFile("/etc/default/keyboard", originalKeyboard, keyboardInfo.Mode().Perm())
	})

	desiredTimezone := "Europe/Berlin"
	if originalTimezone == desiredTimezone {
		desiredTimezone = "UTC"
	}
	timezoneProvider := vmHostLocaleProvider(t, models.HostLocaleResource{Name: "timezone", Timezone: &desiredTimezone})
	if check := timezoneProvider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("timezone drift Check = %+v, want drifted", check)
	}
	if result := timezoneProvider.Apply(ctx); result.Status != contract.Changed || result.Err != nil || len(result.Activation) != 0 || result.RebootRequired != contract.RebootNotRequired {
		t.Fatalf("timezone Apply = %+v, want immediate changed outcome", result)
	}
	if got := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value"); got != desiredTimezone {
		t.Fatalf("timezone = %q, want %q", got, desiredTimezone)
	}
	localeAfterTimezone, keymapAfterTimezone := vmLocalectlState(t)
	if !maps.Equal(localeAfterTimezone, originalLocale) || keymapAfterTimezone != originalKeymap {
		t.Fatalf("timezone-only Apply changed locale/keymap: locale=%v keymap=%q", localeAfterTimezone, keymapAfterTimezone)
	}
	vmAssertHostLocaleSecondCheck(t, timezoneProvider)

	desiredLocale := "C"
	if originalLocale["LANG"] == desiredLocale {
		desiredLocale = "C.UTF-8"
	}
	localeProvider := vmHostLocaleProvider(t, models.HostLocaleResource{Name: "locale", Locale: map[string]string{"LANG": desiredLocale}})
	result := localeProvider.Apply(ctx)
	if result.Status != contract.Changed || result.Err != nil || !hasHostLocaleActivation(result.Activation, contract.ActivationLogoutRequired) || result.RebootRequired != contract.RebootNotRequired {
		t.Fatalf("locale Apply = %+v, want changed/logout-required", result)
	}
	localeAfterLocale, keymapAfterLocale := vmLocalectlState(t)
	if localeAfterLocale["LANG"] != desiredLocale || keymapAfterLocale != originalKeymap {
		t.Fatalf("locale-only Apply state = locale=%v keymap=%q", localeAfterLocale, keymapAfterLocale)
	}
	for key, value := range originalLocale {
		if key != "LANG" && localeAfterLocale[key] != value {
			t.Fatalf("locale-only Apply changed omitted %s from %q to %q", key, value, localeAfterLocale[key])
		}
	}
	if got := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value"); got != desiredTimezone {
		t.Fatalf("locale-only Apply changed timezone to %q", got)
	}
	vmAssertHostLocaleSecondCheck(t, localeProvider)

	desiredKeymap := vmDifferentKeymap(t, keymapAfterLocale)
	keymapProvider := vmHostLocaleProvider(t, models.HostLocaleResource{Name: "keymap", Keymap: &desiredKeymap})
	result = keymapProvider.Apply(ctx)
	if result.Status != contract.Changed || result.Err != nil || !hasHostLocaleActivation(result.Activation, contract.ActivationRebootRequired) || result.RebootRequired != contract.RebootRequired {
		t.Fatalf("keymap Apply = %+v, want changed/reboot-required", result)
	}
	localeAfterKeymap, keymapAfterKeymap := vmLocalectlState(t)
	if !maps.Equal(localeAfterKeymap, localeAfterLocale) || keymapAfterKeymap != desiredKeymap {
		t.Fatalf("keymap-only Apply state = locale=%v keymap=%q", localeAfterKeymap, keymapAfterKeymap)
	}
	if got := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value"); got != desiredTimezone {
		t.Fatalf("keymap-only Apply changed timezone to %q", got)
	}
	vmAssertHostLocaleSecondCheck(t, keymapProvider)

	rollbackTimezone := originalTimezone
	if rollbackTimezone == desiredTimezone {
		rollbackTimezone = "America/Los_Angeles"
	}
	invalidProvider := vmHostLocaleProvider(t, models.HostLocaleResource{
		Name: "native-invalid", Timezone: &rollbackTimezone, Locale: map[string]string{"LANG": "zz_ZZ.UTF-8"},
	})
	result = invalidProvider.Apply(ctx)
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("native-invalid locale Apply = %+v, want failed", result)
	}
	if got := vmHostLocaleValue(t, "timedatectl", "show", "--property=Timezone", "--value"); got != desiredTimezone {
		t.Fatalf("failed locale Apply left timezone %q, want restored %q", got, desiredTimezone)
	}
	localeAfterFailure, keymapAfterFailure := vmLocalectlState(t)
	if !maps.Equal(localeAfterFailure, localeAfterKeymap) || keymapAfterFailure != desiredKeymap {
		t.Fatalf("failed locale Apply changed preserved state: locale=%v keymap=%q", localeAfterFailure, keymapAfterFailure)
	}
}

func vmHostLocaleProvider(t *testing.T, resource models.HostLocaleResource) contract.Provider {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(hostlocale.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmHostLocaleValue(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmLocalectlState(t *testing.T) (map[string]string, string) {
	t.Helper()
	output := vmHostLocaleValue(t, "localectl", "status", "--no-pager")
	locale := make(map[string]string)
	readingLocale := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(trimmed, "System Locale:"); ok {
			readingLocale = true
			vmParseLocaleAssignments(locale, value)
			continue
		}
		if readingLocale && strings.HasPrefix(line, " ") {
			vmParseLocaleAssignments(locale, trimmed)
			continue
		}
		readingLocale = false
	}
	return locale, vmKeyboardLayout(t)
}

func vmKeyboardLayout(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("/etc/default/keyboard")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "XKBLAYOUT" {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	t.Fatal("/etc/default/keyboard does not declare XKBLAYOUT")
	return ""
}

func vmParseLocaleAssignments(locale map[string]string, value string) {
	for _, field := range strings.Fields(value) {
		key, item, ok := strings.Cut(field, "=")
		if ok {
			locale[key] = item
		}
	}
}

func vmLocaleAssignments(locale map[string]string) []string {
	keys := make([]string, 0, len(locale))
	for key := range locale {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assignments := make([]string, 0, len(keys))
	for _, key := range keys {
		assignments = append(assignments, key+"="+locale[key])
	}
	return assignments
}

func vmDifferentKeymap(t *testing.T, current string) string {
	t.Helper()
	for _, candidate := range []string{"us", "de"} {
		if candidate != current {
			if output, err := exec.Command("ckbcomp", candidate).CombinedOutput(); err != nil {
				t.Fatalf("ckbcomp %s: %v: %s", candidate, err, output)
			}
			return candidate
		}
	}
	t.Fatalf("Ubuntu console-setup did not expose a different us/de keymap; current=%q", current)
	return ""
}

func vmAssertHostLocaleSecondCheck(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("host-locale second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant host-locale Apply = %+v, want no change", result)
	}
}

func hasHostLocaleActivation(signals []contract.ActivationSignal, want contract.ActivationKind) bool {
	for _, signal := range signals {
		if signal.Kind == want {
			return true
		}
	}
	return false
}
