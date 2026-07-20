package configrepo

import (
	"strings"
	"testing"
)

// OS-PRM-001, OS-PRM-002, OS-PRM-005, OS-PRM-006: the public configuration
// seam rejects lifecycle and transaction policy that the selected provider
// cannot truthfully converge.
func TestValidateRepositoryPackageLifecycleAndPolicyByProvider(t *testing.T) {
	tests := []struct {
		name        string
		packageYAML string
		wantIssue   string
	}{
		{
			name: "APT exact present policy",
			packageYAML: `name: curl
        present: true
        packageManager: apt
        version: 2.0.0-1
        allowUpgrade: true
        allowDowngrade: true
        hold: true
        refreshCache: true`,
		},
		{
			name: "APT absent dependency cleanup",
			packageYAML: `name: curl
        present: false
        packageManager: apt
        removeDependencies: true`,
		},
		{
			name: "APT purge",
			packageYAML: `name: curl
        lifecycle: purged
        packageManager: apt`,
		},
		{
			name: "Pacman exact present policy",
			packageYAML: `name: curl
        present: true
        packageManager: pacman
        version: 2.0.0-1
        allowUpgrade: true
        allowDowngrade: true
        refreshCache: true`,
		},
		{
			name: "Pacman absent dependency cleanup",
			packageYAML: `name: curl
        present: false
        packageManager: pacman
        removeDependencies: true`,
		},
		{
			name: "Pacman purge is unsupported",
			packageYAML: `name: curl
        lifecycle: purged
        packageManager: pacman`,
			wantIssue: `lifecycle "purged" is unsupported by packageManager pacman`,
		},
		{
			name: "upgrade policy requires exact version",
			packageYAML: `name: curl
        present: true
        packageManager: apt
        allowUpgrade: false`,
			wantIssue: "allowUpgrade requires an exact version",
		},
		{
			name: "absent package rejects exact version",
			packageYAML: `name: curl
        present: false
        packageManager: apt
        version: 2.0.0-1`,
			wantIssue: "version is valid only for lifecycle present",
		},
		{
			name: "multiple absent fields report deterministic first violation",
			packageYAML: `name: curl
        present: false
        packageManager: apt
        version: 2.0.0-1
        allowUpgrade: true
        hold: true`,
			wantIssue: "version is valid only for lifecycle present",
		},
		{
			name: "absent package rejects hold",
			packageYAML: `name: curl
        present: false
        packageManager: apt
        hold: true`,
			wantIssue: "hold is valid only for lifecycle present",
		},
		{
			name: "absent package rejects refresh",
			packageYAML: `name: curl
        present: false
        packageManager: apt
        refreshCache: true`,
			wantIssue: "refreshCache is valid only for lifecycle present",
		},
		{
			name: "present package rejects removal policy",
			packageYAML: `name: curl
        present: true
        packageManager: apt
        removeDependencies: true`,
			wantIssue: "removeDependencies is valid only for lifecycle absent or purged",
		},
		{
			name: "non-native provider rejects downgrade policy",
			packageYAML: `name: org.example.App
        present: true
        packageManager: flatpak
        allowDowngrade: true`,
			wantIssue: "allowDowngrade is unsupported by packageManager flatpak",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFleetModule(t, dir, "engineering", "configurations:\n  - name: base\n    packages:\n      - "+test.packageYAML+"\n")
			result, err := ValidateRepository(dir)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantIssue == "" {
				if len(result.Issues) != 0 {
					t.Fatalf("issues = %+v, want valid provider policy", result.Issues)
				}
				return
			}
			if len(result.Issues) != 1 || !strings.Contains(result.Issues[0].Message, test.wantIssue) {
				t.Fatalf("issues = %+v, want one containing %q", result.Issues, test.wantIssue)
			}
		})
	}
}
