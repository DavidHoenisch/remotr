//go:build vmsafety

package systemd

import (
	"errors"
	"os"
	"testing"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestSystemdProviderUbuntu2604VM(t *testing.T) {
	const unit = "remotr-core-delivery-qualification.service"
	const unitPath = "/run/systemd/system/" + unit
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=Remotr qualification\n[Service]\nType=simple\nExecStart=/usr/bin/sleep infinity\n[Install]\nWantedBy=multi-user.target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _, _ = New(models.SystemdResource{Name: "cleanup", Unit: unit}, nil).Exec.Run("systemctl", "disable", "--now", unit)
		_ = os.Remove(unitPath)
		_, _, _ = New(models.SystemdResource{Name: "cleanup", Unit: unit}, nil).Exec.Run("systemctl", "daemon-reload")
	})
	enabled, active := true, true
	provider := New(models.SystemdResource{Name: "qualification", Unit: unit, Enabled: &enabled, Active: &active}, nil)
	if result := provider.Check(t.Context()); result.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", result)
	}
	if err := provider.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if result := provider.Check(t.Context()); result.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", result)
	}
	if err := provider.Apply(t.Context()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v", err)
	}
}
