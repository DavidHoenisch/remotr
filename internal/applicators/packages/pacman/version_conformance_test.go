package pacman_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const (
	pacmanPackage       = "contract-package"
	pacmanV1            = "1.0.0-1"
	pacmanV2            = "2.0.0-1"
	pacmanV2ArtifactURL = "file:///repo/contract-package-2.0.0-1-x86_64.pkg.tar.zst"
)

// OS-PRM-027 and OS-PRM-028: exact Pacman intent installs only the artifact
// whose repository resolution proves the declared package name and version.
func TestApplicator_ConformsForExactResolvedArtifact(t *testing.T) {
	allow := true
	runner := &resolvedArtifactRunner{
		installed: pacmanV1,
		available: pacmanV2,
		resolved:  resolvedPacmanArtifact{name: pacmanPackage, version: pacmanV2, location: pacmanV2ArtifactURL},
	}
	provider := newVersionContractProvider(t, models.Package{
		Name: pacmanPackage, Present: true, Version: pacmanV2, AllowUpgrade: &allow,
	}, runner)

	if check := provider.Check(t.Context()); check.Status != contract.Drifted {
		t.Fatalf("initial Check() = %+v, want drift", check)
	}
	result := provider.Apply(t.Context())
	if result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("Apply() = %+v, want changed", result)
	}
	if runner.installed != pacmanV2 {
		t.Fatalf("installed version = %q, want %q", runner.installed, pacmanV2)
	}
	if check := provider.Check(t.Context()); check.Status != contract.Compliant {
		t.Fatalf("second Check() = %+v, want compliant", check)
	}
	if result := provider.Apply(t.Context()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("second Apply() = %+v, want no-change", result)
	}
	if !slices.Equal(runner.installs, []string{pacmanV2ArtifactURL}) {
		t.Fatalf("installed artifacts = %q, want exact resolved artifact", runner.installs)
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && slices.Contains(call.Args, "-S") {
			t.Fatalf("exact-version provider issued unversioned transaction: %+v", call)
		}
	}
}

func TestApplicator_FailsClosedWhenRepositoryResolutionChanges(t *testing.T) {
	tests := []struct {
		name     string
		resolved resolvedPacmanArtifact
	}{
		{name: "lower version", resolved: resolvedPacmanArtifact{name: pacmanPackage, version: pacmanV1, location: "file:///repo/lower.pkg.tar.zst"}},
		{name: "higher version", resolved: resolvedPacmanArtifact{name: pacmanPackage, version: "9.0.0-1", location: "file:///repo/higher.pkg.tar.zst"}},
		{name: "lower name", resolved: resolvedPacmanArtifact{name: "aaa-package", version: pacmanV2, location: "file:///repo/lower-name.pkg.tar.zst"}},
		{name: "higher name", resolved: resolvedPacmanArtifact{name: "zzz-package", version: pacmanV2, location: "file:///repo/higher-name.pkg.tar.zst"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &resolvedArtifactRunner{installed: pacmanV1, available: pacmanV2, resolved: test.resolved}
			provider := newVersionContractProvider(t, models.Package{Name: pacmanPackage, Present: true, Version: pacmanV2}, runner)

			result := provider.Apply(t.Context())
			if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "resolv") {
				t.Fatalf("changed resolution Apply() = %+v, want fail-closed resolution error", result)
			}
			if runner.installed != pacmanV1 || len(runner.installs) != 0 {
				t.Fatalf("changed resolution mutated package database: installed=%q transactions=%q", runner.installed, runner.installs)
			}
		})
	}
}

// OS-PRM-003 through OS-PRM-005: Pacman uses provider-native version
// comparison and leaves the controlled package database unchanged whenever a
// requested transition is unavailable, prohibited, or fails at installation.
func TestApplicator_ConformsForExactVersionPolicy(t *testing.T) {
	allow, deny := true, false
	installFailure := errors.New("pacman install failed")
	tests := []struct {
		name           string
		installed      string
		available      string
		requested      string
		allowUpgrade   *bool
		allowDowngrade *bool
		installErr     error
		comparison     string
		wantStatus     contract.ApplyStatus
		wantInstalled  string
	}{
		{name: "exact upgrade", installed: pacmanV1, available: pacmanV2, requested: pacmanV2, wantStatus: contract.Changed, wantInstalled: pacmanV2},
		{name: "exact install from absence", available: pacmanV2, requested: pacmanV2, wantStatus: contract.Changed, wantInstalled: pacmanV2},
		{name: "unavailable version", installed: pacmanV1, available: pacmanV2, requested: "9.9.9-1", wantStatus: contract.Failed, wantInstalled: pacmanV1},
		{name: "upgrade blocked", installed: pacmanV1, available: pacmanV2, requested: pacmanV2, allowUpgrade: &deny, wantStatus: contract.Failed, wantInstalled: pacmanV1},
		{name: "downgrade blocked", installed: pacmanV2, available: pacmanV1, requested: pacmanV1, wantStatus: contract.Failed, wantInstalled: pacmanV2},
		{name: "permitted downgrade", installed: pacmanV2, available: pacmanV1, requested: pacmanV1, allowUpgrade: &deny, allowDowngrade: &allow, wantStatus: contract.Changed, wantInstalled: pacmanV1},
		{name: "native-equivalent exact version", installed: "1.0.0-01", available: pacmanV1, requested: pacmanV1, allowUpgrade: &deny, allowDowngrade: &deny, comparison: "0", wantStatus: contract.Changed, wantInstalled: pacmanV1},
		{name: "failed exact install", installed: pacmanV1, available: pacmanV2, requested: pacmanV2, installErr: installFailure, wantStatus: contract.Failed, wantInstalled: pacmanV1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := fmt.Sprintf("file:///repo/%s-%s-x86_64.pkg.tar.zst", pacmanPackage, tt.available)
			runner := &resolvedArtifactRunner{
				installed:  tt.installed,
				available:  tt.available,
				resolved:   resolvedPacmanArtifact{name: pacmanPackage, version: tt.available, location: location},
				installErr: tt.installErr,
				comparison: tt.comparison,
			}
			provider := newVersionContractProvider(t, models.Package{
				Name: pacmanPackage, Present: true, Version: tt.requested,
				AllowUpgrade: tt.allowUpgrade, AllowDowngrade: tt.allowDowngrade,
			}, runner)

			if check := provider.Check(t.Context()); check.Status != contract.Drifted {
				t.Fatalf("initial Check() = %+v, want drift", check)
			}
			result := provider.Apply(t.Context())
			if result.Status != tt.wantStatus {
				t.Fatalf("Apply() = %+v, want %q", result, tt.wantStatus)
			}
			if runner.installed != tt.wantInstalled {
				t.Fatalf("installed version = %q, want %q", runner.installed, tt.wantInstalled)
			}
			if tt.requested != tt.available {
				for _, call := range runner.calls {
					if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] == "-Sp" {
						t.Fatalf("unavailable version reached artifact resolution: %+v", call)
					}
				}
			}
			second := provider.Check(t.Context())
			if tt.wantStatus == contract.Changed {
				if second.Status != contract.Compliant {
					t.Fatalf("second Check() = %+v, want compliant", second)
				}
			} else if second.Status != contract.Drifted {
				t.Fatalf("failed transition Check() = %+v, want original drift", second)
			}
		})
	}
}

func newVersionContractProvider(t *testing.T, pkg models.Package, runner *resolvedArtifactRunner) contract.Provider {
	t.Helper()
	provider, err := contract.New(pacman.New(pkg, runner))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type resolvedPacmanArtifact struct {
	name     string
	version  string
	location string
}

type resolvedArtifactRunner struct {
	installed  string
	available  string
	resolved   resolvedPacmanArtifact
	installErr error
	comparison string
	installs   []string
	calls      []executil.MockCall
}

func (r *resolvedArtifactRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	switch {
	case name == "pacman" && slices.Equal(args, []string{"-Q", pacmanPackage}):
		if r.installed == "" {
			return nil, nil, errors.New("package is not installed")
		}
		return []byte(pacmanPackage + " " + r.installed + "\n"), nil, nil
	case name == "pacman" && slices.Equal(args, []string{"-Si", pacmanPackage}):
		return []byte("Name : " + pacmanPackage + "\nVersion : " + r.available + "\n"), nil, nil
	case name == "pacman" && slices.Equal(args, []string{"-Sp", "--print-format", "%n\t%v\t%l", pacmanPackage}):
		return []byte(fmt.Sprintf("%s\t%s\t%s\n", r.resolved.name, r.resolved.version, r.resolved.location)), nil, nil
	case name == "vercmp" && len(args) == 2:
		if r.comparison != "" {
			return []byte(r.comparison + "\n"), nil, nil
		}
		switch {
		case args[0] == args[1]:
			return []byte("0\n"), nil, nil
		case args[0] == pacmanV1 && args[1] == pacmanV2:
			return []byte("-1\n"), nil, nil
		case args[0] == pacmanV2 && args[1] == pacmanV1:
			return []byte("1\n"), nil, nil
		default:
			return nil, nil, fmt.Errorf("unknown version comparison %q %q", args[0], args[1])
		}
	case name == "pacman" && len(args) == 3 && args[0] == "-U" && args[1] == "--noconfirm":
		if args[2] != r.resolved.location {
			return nil, nil, fmt.Errorf("unexpected artifact %q", args[2])
		}
		if r.installErr != nil {
			return nil, []byte("controlled exact install failure"), r.installErr
		}
		r.installed = r.resolved.version
		r.installs = append(r.installs, args[2])
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
}
