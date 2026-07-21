package models_test

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func FuzzParseCanonicalPackagePolicy(f *testing.F) {
	f.Add("apt", "present", "2.0.0-1", "x86", uint16(0b1111111111), "")
	f.Add("pacman", "absent", "", "x86", uint16(0b0010000000), "")
	f.Add("pacman", "purged", "", "ARM", uint16(0), "")
	f.Add("yay", "present", "1.0.0-1", "x86", uint16(0b0000001111), "bounded-cache")
	f.Add("unknown", "present", "", "unknown", uint16(0), "option")

	f.Fuzz(func(t *testing.T, provider, lifecycle, version, architecture string, flags uint16, providerOption string) {
		if len(provider) > 32 || len(lifecycle) > 32 || len(version) > 128 || len(architecture) > 32 || len(providerOption) > 256 {
			return
		}
		if !utf8.ValidString(provider) || !utf8.ValidString(lifecycle) || !utf8.ValidString(version) || !utf8.ValidString(architecture) || !utf8.ValidString(providerOption) {
			return
		}
		var input strings.Builder
		fmt.Fprintf(&input, "schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: package\n        name: fixture\n        lifecycle: %s\n        packageManager: %s\n        version: %s\n        arch: %s\n",
			strconv.Quote(lifecycle), strconv.Quote(provider), strconv.Quote(version), strconv.Quote(architecture))
		appendFuzzBool(&input, "allowUpgrade", flags, 0, 1)
		appendFuzzBool(&input, "allowDowngrade", flags, 2, 3)
		appendFuzzBool(&input, "hold", flags, 4, 5)
		fmt.Fprintf(&input, "        refreshCache: %t\n        removeDependencies: %t\n", flags&(1<<6) != 0, flags&(1<<7) != 0)
		appendFuzzBool(&input, "nonInteractive", flags, 8, 9)
		if provider == "flatpak" {
			fmt.Fprintf(&input, "        flatpakRemote: %s\n", strconv.Quote(providerOption))
		}
		if provider == "pwa" {
			fmt.Fprintf(&input, "        pwaURL: %s\n", strconv.Quote(providerOption))
		}
		if provider == "yay" {
			fmt.Fprintf(&input, "        aurBuildUser: %s\n", strconv.Quote(providerOption))
		}

		state, err := models.ParseState(strings.NewReader(input.String()))
		if err != nil {
			if len(err.Error()) > 1024 {
				t.Fatalf("package parser diagnostic is unbounded: %d bytes", len(err.Error()))
			}
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != 1 {
			t.Fatalf("parsed state = %#v, want one package", state)
		}
		pkg := state.Configurations[0].Packages[0]
		if pkg.Present != (pkg.Lifecycle == models.LifecyclePresent) {
			t.Fatalf("lifecycle normalization mismatch: lifecycle=%q present=%t", pkg.Lifecycle, pkg.Present)
		}
		if string(pkg.Lifecycle) != lifecycle || string(pkg.PM) != provider || pkg.Version != version || string(pkg.Arch) != architecture {
			t.Fatalf("package dimensions changed: %+v", pkg)
		}
		if provider == "yay" && pkg.AURBuildUser != providerOption {
			t.Fatalf("AUR build identity changed: got %q want %q", pkg.AURBuildUser, providerOption)
		}

		canonical, err := resourceregistry.MarshalCanonical(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := models.ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical package did not parse: %v", err)
		}
		recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(recanonical, canonical) {
			t.Fatalf("canonical package changed after parse round trip:\nfirst:\n%s\nsecond:\n%s", canonical, recanonical)
		}
	})
}

func appendFuzzBool(output *strings.Builder, field string, flags uint16, presentBit, valueBit uint) {
	if flags&(1<<presentBit) == 0 {
		return
	}
	fmt.Fprintf(output, "        %s: %t\n", field, flags&(1<<valueBit) != 0)
}
