package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-PRM-026: canonical AUR authoring exposes only typed package intent and a
// declared build identity; executable input and generic provider escape
// hatches remain outside the schema.
func TestParseStateCanonicalAURPackageIntent(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: remotr-aur-fixture
        lifecycle: present
        packageManager: yay
        version: 1.0.0-1
        allowUpgrade: true
        allowDowngrade: false
        nonInteractive: true
        aurBuildUser: remotr-build
`))
	if err != nil {
		t.Fatalf("ParseState() = %v, want typed AUR intent", err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != 1 {
		t.Fatalf("ParseState() = %#v, want one AUR package", state)
	}
	pkg := state.Configurations[0].Packages[0]
	if pkg.Name != "remotr-aur-fixture" || pkg.AURBuildUser != "remotr-build" || pkg.Version != "1.0.0-1" {
		t.Fatalf("AUR package intent = %+v", pkg)
	}
}

func TestParseStateRejectsArbitraryAURBuildInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "shell command", input: "command: [sh, -c, id]"},
		{name: "PKGBUILD body", input: "pkgbuild: 'package() { :; }'"},
		{name: "arbitrary build flags", input: "buildFlags: [--mflags, -j64]"},
		{name: "generic provider escape hatch", input: "providerOptions:\n          yay:\n            command: [sh, -c, id]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: remotr-aur-fixture
        lifecycle: present
        packageManager: yay
        aurBuildUser: remotr-build
        ` + tt.input + "\n"
			if _, err := models.ParseState(strings.NewReader(input)); err == nil {
				t.Fatalf("ParseState() accepted arbitrary AUR input %q", tt.input)
			}
		})
	}
}
