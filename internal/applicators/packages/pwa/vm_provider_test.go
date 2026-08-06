//go:build vmsafety

package pwa_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pwa"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestPWAProviderUbuntu2604VM(t *testing.T) {
	for _, browserName := range []string{"chromium", "google-chrome-stable"} {
		t.Run(browserName, func(t *testing.T) { exercisePWAProviderCoreDeliveryVM(t, browserName) })
	}
}

func TestPWAProviderPopOS2404VM(t *testing.T) {
	exercisePWAProviderCoreDeliveryVM(t, "chromium")
}

func exercisePWAProviderCoreDeliveryVM(t *testing.T, browserName string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	browser := filepath.Join(bin, browserName)
	if err := os.WriteFile(browser, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	account := interactiveuser.Account{Username: "root", UID: os.Getuid(), GID: os.Getgid(), HomeDir: filepath.Join(dir, "home")}
	if err := os.Mkdir(account.HomeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pkg := models.Package{
		Name: "qualification", Present: true, PM: types.Pwa,
		PWAURL: "https://qualification.invalid/app", PWATitle: "Remotr Qualification",
		PWABrowser: browserName,
	}
	provider := pwa.New(pkg, nil)
	provider.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{account}, nil }
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
	desktop := filepath.Join(account.HomeDir, ".local", "share", "applications", "remotr-pwa-qualification.desktop")
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"Name=Remotr Qualification", browser + " --app=https://qualification.invalid/app", "StartupWMClass=qualification.invalid",
	} {
		if !strings.Contains(string(data), fragment) {
			t.Errorf("desktop entry omits %q: %q", fragment, data)
		}
	}

	pkg.Present = false
	provider = pwa.New(pkg, nil)
	provider.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{account}, nil }
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(desktop); !os.IsNotExist(err) {
		t.Fatalf("desktop entry remains after absent apply: %v", err)
	}
}
