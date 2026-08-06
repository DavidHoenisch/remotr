//go:build vmsafety

package flatpak_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/flatpak"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestFlatpakProviderUbuntu2604VM(t *testing.T) {
	testFlatpakProviderCoreDeliveryVM(t)
}

func TestFlatpakProviderPopOS2404VM(t *testing.T) {
	testFlatpakProviderCoreDeliveryVM(t)
}

func testFlatpakProviderCoreDeliveryVM(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$REMOTR_FLATPAK_LOG"
case "$1" in
  info) test -f "$REMOTR_FLATPAK_INSTALLED" ;;
  remote-list) test ! -f "$REMOTR_FLATPAK_REMOTE" || printf '%s\n' qualification ;;
  remote-add) test "$*" = "remote-add --if-not-exists qualification https://qualification.invalid/repo.flatpakrepo"; : > "$REMOTR_FLATPAK_REMOTE" ;;
  install) test "$*" = "install --assumeyes --noninteractive qualification org.remotr.Qualification"; : > "$REMOTR_FLATPAK_INSTALLED" ;;
  uninstall) test "$*" = "uninstall --assumeyes --noninteractive org.remotr.Qualification"; rm -f "$REMOTR_FLATPAK_INSTALLED" ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "flatpak"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "argv.log")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("REMOTR_FLATPAK_LOG", logPath)
	t.Setenv("REMOTR_FLATPAK_REMOTE", filepath.Join(dir, "remote"))
	t.Setenv("REMOTR_FLATPAK_INSTALLED", filepath.Join(dir, "installed"))

	pkg := models.Package{
		Name: "org.remotr.Qualification", Present: true, PM: types.Flatpak,
		FlatpakRemote: "qualification", FlatpakRemoteURL: "https://qualification.invalid/repo.flatpakrepo",
	}
	provider := flatpak.New(pkg, nil)
	if _, compliant := provider.State(context.Background()); compliant {
		t.Fatal("initial state unexpectedly compliant")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("state remained drifted after apply")
	}
	if err := provider.Apply(context.Background()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v", err)
	}

	pkg.Present = false
	provider = flatpak.New(pkg, nil)
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("absent state remained drifted after apply")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, argv := range []string{
		"remote-add --if-not-exists qualification https://qualification.invalid/repo.flatpakrepo",
		"install --assumeyes --noninteractive qualification org.remotr.Qualification",
		"uninstall --assumeyes --noninteractive org.remotr.Qualification",
	} {
		if !strings.Contains(string(data), argv+"\n") {
			t.Errorf("argv log omits %q: %q", argv, data)
		}
	}
}
