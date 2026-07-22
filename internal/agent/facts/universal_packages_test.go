package facts

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestDetectLocalBackendsReportsObservedPortablePackageProviders(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"flatpak", "chromium"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	got := detectLocalBackends(Facts{}).Normalized()
	if !reflect.DeepEqual(got.UniversalPackage, []types.PackageManager{types.Flatpak}) {
		t.Fatalf("universal package providers = %v, want [flatpak]", got.UniversalPackage)
	}
	if !reflect.DeepEqual(got.Browser, []BrowserBackend{BrowserChromium}) {
		t.Fatalf("browser providers = %v, want [chromium]", got.Browser)
	}
}
