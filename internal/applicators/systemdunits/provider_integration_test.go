//go:build providerintegration

package systemdunits_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemdunits"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicatorUsesRealSystemdAnalyzeVerification(t *testing.T) {
	if _, err := os.Stat("/usr/bin/systemd-analyze"); err != nil {
		// test-exception: EXC-018
		t.Skip("systemd-analyze is required")
	}
	t.Run("unit", func(t *testing.T) {
		unitDir := t.TempDir()
		provider := systemdunits.New(models.SystemdUnitResource{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "provider-unit", Unit: "provider-unit.service",
			Content: "[Unit]\nDescription=Remotr provider verification\n[Service]\nType=oneshot\nExecStart=/usr/bin/true\n",
		}, nil)
		provider.UnitDir = unitDir
		provider.LookupOwner = func(string, string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
		if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
			t.Fatalf("ApplyResult() = %+v", result)
		}
	})
	t.Run("drop-in", func(t *testing.T) {
		unitDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(unitDir, "provider-dropin.service"), []byte("[Service]\nType=oneshot\nExecStart=/usr/bin/true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		provider := systemdunits.New(models.SystemdUnitResource{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "provider-dropin", Unit: "provider-dropin.service", DropIn: "20-remotr.conf",
			Content: "[Service]\nTimeoutStartSec=30s\n",
		}, nil)
		provider.UnitDir = unitDir
		provider.LookupOwner = func(string, string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
		if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
			t.Fatalf("ApplyResult() = %+v", result)
		}
	})
}
