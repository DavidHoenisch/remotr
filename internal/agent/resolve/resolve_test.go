package resolve_test

import (
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestResolve_filtersByDistroAndArch(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{
		{
			Name:          "debian-only",
			TargetDistros: []types.Distro{types.Debian},
			Packages:      []models.Package{{Name: "curl", Present: true, PM: types.Apt}},
		},
		{
			Name:          "arch-only",
			TargetDistros: []types.Distro{types.Arch},
			Packages:      []models.Package{{Name: "curl", Present: true, PM: types.Pacman}},
		},
		{
			Name:       "x86-only",
			TargetArch: []types.Architecture{types.X86},
			Users:      []models.UserResource{{Name: "dev", Username: "dev", Present: true}},
		},
		{
			Name: "universal",
			Files: []models.File{
				{Name: "motd", Path: "/etc/motd", Content: "hello"},
			},
		},
	}}

	tests := []struct {
		name string
		f    facts.Facts
		want []string
	}{
		{
			name: "debian x86",
			f:    facts.Facts{Distro: types.Debian, Arch: types.X86},
			want: []string{"debian-only", "x86-only", "universal"},
		},
		{
			name: "arch arm",
			f:    facts.Facts{Distro: types.Arch, Arch: types.Arm},
			want: []string{"arch-only", "universal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve.Resolve(state, tt.f)
			var names []string
			for _, c := range got.Configurations {
				names = append(names, c.Name)
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Fatalf("config names = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestResolve_filtersPackagesByPM(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{{
		Name:          "mixed",
		TargetDistros: []types.Distro{types.Debian, types.Arch},
		Packages: []models.Package{
			{Name: "curl", Present: true, PM: types.Apt},
			{Name: "curl", Present: true, PM: types.Pacman},
		},
	}}}

	got := resolve.Resolve(state, facts.Facts{Distro: types.Debian, Arch: types.X86})
	if len(got.Configurations) != 1 || len(got.Configurations[0].Packages) != 1 {
		t.Fatalf("expected one apt package, got %#v", got)
	}
	if got.Configurations[0].Packages[0].PM != types.Apt {
		t.Fatalf("expected apt package")
	}
}
