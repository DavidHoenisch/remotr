package apt_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-PRM-003 through OS-PRM-005: exact APT versions use provider-native
// comparison and preserve the package database for unavailable or prohibited
// transitions.
func TestApplicator_ConformsForExactVersionPolicy(t *testing.T) {
	allow, deny := true, false
	tests := []struct {
		name           string
		installed      string
		available      []string
		requested      string
		allowUpgrade   *bool
		allowDowngrade *bool
		wantStatus     contract.ApplyStatus
		wantInstalled  string
	}{
		{name: "exact upgrade", installed: "1.0", available: []string{"1.0", "2.0"}, requested: "2.0", wantStatus: contract.Changed, wantInstalled: "2.0"},
		{name: "unavailable version", installed: "1.0", available: []string{"1.0", "2.0"}, requested: "3.0", wantStatus: contract.Failed, wantInstalled: "1.0"},
		{name: "upgrade blocked", installed: "1.0", available: []string{"1.0", "2.0"}, requested: "2.0", allowUpgrade: &deny, wantStatus: contract.Failed, wantInstalled: "1.0"},
		{name: "downgrade blocked", installed: "2.0", available: []string{"1.0", "2.0"}, requested: "1.0", wantStatus: contract.Failed, wantInstalled: "2.0"},
		{name: "permitted downgrade", installed: "2.0", available: []string{"1.0", "2.0"}, requested: "1.0", allowDowngrade: &allow, wantStatus: contract.Changed, wantInstalled: "1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &aptVersionRunner{installed: tt.installed, available: append([]string(nil), tt.available...)}
			provider, err := contract.New(apt.New(models.Package{
				Name: "contract-package", Present: true, Version: tt.requested,
				AllowUpgrade: tt.allowUpgrade, AllowDowngrade: tt.allowDowngrade,
			}, runner))
			if err != nil {
				t.Fatal(err)
			}

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

type aptVersionRunner struct {
	installed string
	available []string
}

func (r *aptVersionRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	switch {
	case name == "dpkg-query" && slices.Equal(args, []string{"-W", "-f=${Status}\\t${Version}", "contract-package"}):
		if r.installed == "" {
			return nil, nil, errors.New("package is not installed")
		}
		return []byte("install ok installed\t" + r.installed + "\n"), nil, nil
	case name == "dpkg" && len(args) == 4 && args[0] == "--compare-versions" && args[2] == "gt":
		if knownAPTVersionGreater(args[1], args[3]) {
			return nil, nil, nil
		}
		return nil, nil, errors.New("comparison is false")
	case name == "apt-get" && len(args) >= 3 && args[0] == "install" && args[1] == "-y":
		target := args[len(args)-1]
		packageName, version, ok := strings.Cut(target, "=")
		if !ok || packageName != "contract-package" || !slices.Contains(r.available, version) {
			return nil, []byte("requested version is unavailable"), errors.New("apt-get exited 100")
		}
		if knownAPTVersionGreater(r.installed, version) && !slices.Contains(args, "--allow-downgrades") {
			return nil, []byte("downgrade requires explicit authorization"), errors.New("apt-get exited 100")
		}
		r.installed = version
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
}

func knownAPTVersionGreater(left, right string) bool {
	return (left == "2.0" && right == "1.0") || (left == "3.0" && right != "3.0")
}
