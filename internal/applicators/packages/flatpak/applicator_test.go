package flatpak_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/flatpak"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestApplicator_installed(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"flatpak [info org.gnome.Calculator]": {Err: nil},
		},
	}
	a := flatpak.New(models.Package{Name: "org.gnome.Calculator", Present: true, PM: types.Flatpak}, mock)
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected app present")
	}
}

func TestApplicator_driftWhenAbsent(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"flatpak [info org.gnome.Calculator]": {Err: fmt.Errorf("not installed")},
		},
	}
	a := flatpak.New(models.Package{Name: "org.gnome.Calculator", Present: true, PM: types.Flatpak}, mock)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift")
	}
}

func TestApplicator_applyInstallFromFlathub(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"flatpak [info org.gnome.Calculator]": {Err: fmt.Errorf("missing")},
			"flatpak [remote-list --columns=name]": {Stdout: []byte("flathub\n")},
			"flatpak [install --assumeyes --noninteractive flathub org.gnome.Calculator]": {Err: nil},
		},
	}
	a := flatpak.New(models.Package{Name: "org.gnome.Calculator", Present: true, PM: types.Flatpak}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(mock.Calls) < 3 {
		t.Fatalf("expected remote check and install calls, got %v", mock.Calls)
	}
}

func TestApplicator_applyInstallAddsCustomRemote(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"flatpak [info com.example.App]": {Err: fmt.Errorf("missing")},
			"flatpak [remote-list --columns=name]": {Stdout: []byte("flathub\n")},
			"flatpak [remote-add --if-not-exists company https://store.example.com/repo.flatpakrepo]": {Err: nil},
			"flatpak [install --assumeyes --noninteractive company com.example.App]": {Err: nil},
		},
	}
	a := flatpak.New(models.Package{
		Name:             "com.example.App",
		Present:          true,
		PM:               types.Flatpak,
		FlatpakRemote:    "company",
		FlatpakRemoteURL: "https://store.example.com/repo.flatpakrepo",
	}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
}

func TestApplicator_applyRemove(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"flatpak [info org.gnome.Calculator]": {Err: nil},
			"flatpak [uninstall --assumeyes --noninteractive org.gnome.Calculator]": {Err: nil},
		},
	}
	a := flatpak.New(models.Package{Name: "org.gnome.Calculator", Present: false, PM: types.Flatpak}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
}
