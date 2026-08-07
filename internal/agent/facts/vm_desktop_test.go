//go:build vmsafety

package facts_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestDesktopProviderFactsVM(t *testing.T) {
	discovered, err := facts.Read()
	if err != nil {
		t.Fatal(err)
	}
	release := testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")
	if discovered.Distro != types.Ubuntu || discovered.DistroVersion != release || discovered.Arch != types.X86 {
		t.Fatalf("platform facts = %+v, want Ubuntu %s x86", discovered, release)
	}
	for _, backend := range []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings} {
		if !slices.Contains(discovered.Desktop, backend) {
			t.Fatalf("desktop facts = %v, missing %q", discovered.Desktop, backend)
		}
	}

	accounts, err := interactiveuser.List()
	if err != nil {
		t.Fatal(err)
	}
	wantHomes := map[string]string{
		"remotr-desktop-a": "/home/remotr-desktop-a",
		"remotr-desktop-b": "/home/remotr-desktop-b",
	}
	for _, account := range accounts {
		if home, wanted := wantHomes[account.Username]; wanted {
			if account.HomeDir != home {
				t.Fatalf("interactive account %s home = %q, want %q", account.Username, account.HomeDir, home)
			}
			delete(wantHomes, account.Username)
		}
	}
	if len(wantHomes) != 0 {
		t.Fatalf("interactive account discovery omitted %v", wantHomes)
	}
}
