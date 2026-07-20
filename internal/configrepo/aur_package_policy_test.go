package configrepo

import (
	"strings"
	"testing"
)

func TestValidateRepositoryAcceptsTypedAURIntent(t *testing.T) {
	dir := t.TempDir()
	writeAURPackagePolicy(t, dir, `version: 1.0.0-1
        allowUpgrade: true
        allowDowngrade: false`)
	result, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %+v, want typed AUR package intent accepted", result.Issues)
	}
}

func TestValidateRepositoryRejectsUnsupportedAURIntent(t *testing.T) {
	tests := []struct {
		name      string
		fields    string
		wantIssue string
	}{
		{name: "hold is repository-provider-only", fields: "hold: true", wantIssue: "hold is unsupported by packageManager yay"},
		{name: "refresh is repository-provider-only", fields: "refreshCache: true", wantIssue: "refreshCache is unsupported by packageManager yay"},
		{
			name: "provider options cannot carry build commands",
			fields: `providerOptions:
          yay:
            command: [sh, -c, id]`,
			wantIssue: "providerOptions are unsupported for packageManager yay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAURPackagePolicy(t, dir, tt.fields)
			result, err := ValidateRepository(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Issues) != 1 || !strings.Contains(result.Issues[0].Message, tt.wantIssue) {
				t.Fatalf("issues = %+v, want one containing %q", result.Issues, tt.wantIssue)
			}
		})
	}
}

func writeAURPackagePolicy(t *testing.T, dir, fields string) {
	t.Helper()
	writeFleetModule(t, dir, "engineering", `configurations:
  - name: base
    packages:
      - name: remotr-aur-fixture
        present: true
        packageManager: yay
        aurBuildUser: remotr-build
        `+fields+"\n")
}
