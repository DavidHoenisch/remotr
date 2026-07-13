//go:build vmsafety

package hostlocale_test

import (
	"context"
	"os"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-008, OS-KHB-010: run systemd's real host localization provider in a
// disposable VM, including the structured new-login signal for locale state.
func TestHostLocaleProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("host-locale VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	current := hostlocale.New(models.HostLocaleResource{Name: "current", Timezone: stringPointer("UTC")}, nil)
	check := current.Check(ctx)
	if check.Status == executor.CheckFailed {
		t.Fatalf("read current timezone = %+v", check)
	}
	desiredTimezone := "Europe/Berlin"
	if check.Status != executor.Compliant {
		desiredTimezone = "UTC"
	}
	timezone := hostlocale.New(models.HostLocaleResource{Name: "timezone", Timezone: &desiredTimezone}, nil)
	if err := timezone.Apply(ctx); err != nil {
		t.Fatalf("Apply() real timezone = %v", err)
	}
	if check := timezone.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("Check() after real timezone Apply = %+v", check)
	}

	locale := "C"
	localeProvider := hostlocale.New(models.HostLocaleResource{Name: "locale", Locale: map[string]string{"LANG": locale}}, nil)
	if check := localeProvider.Check(ctx); check.Status == executor.Compliant {
		locale = "C.UTF-8"
		localeProvider = hostlocale.New(models.HostLocaleResource{Name: "locale", Locale: map[string]string{"LANG": locale}}, nil)
	}
	result := localeProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || !hasActivation(result.Activation, executor.ActivationLogoutRequired) {
		t.Fatalf("ApplyResult() real locale = %+v", result)
	}
}

func stringPointer(value string) *string { return &value }

func hasActivation(signals []executor.ActivationSignal, want executor.ActivationKind) bool {
	for _, signal := range signals {
		if signal.Kind == want {
			return true
		}
	}
	return false
}
