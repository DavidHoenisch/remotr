package apt_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_APTProcessBoundary(t *testing.T) {
	allow := true
	hold, unhold := true, false
	missing := errors.New("package is not installed")
	compareFalse := errors.New("comparison is false")
	const literalName = "fixture; echo must-stay-one-argument"

	tests := []struct {
		name   string
		pkg    models.Package
		next   map[string]executil.MockResult
		action func(*apt.Applicator, context.Context) error
		want   []executil.MockCall
	}{
		{
			name: "presence query", pkg: models.Package{Name: "curl", Present: true},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}},
			action: func(a *apt.Applicator, ctx context.Context) error { _, _ = a.State(ctx); return nil },
			want:   []executil.MockCall{{Name: "dpkg", Args: []string{"-s", "curl"}}},
		},
		{
			name: "exact version query", pkg: models.Package{Name: "curl", Present: true, Version: "2.0"},
			next:   map[string]executil.MockResult{"dpkg-query [-W -f=${Status}\\t${Version} curl]": {Stdout: []byte("install ok installed\t2.0\n")}},
			action: func(a *apt.Applicator, ctx context.Context) error { _, _ = a.State(ctx); return nil },
			want:   []executil.MockCall{{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}}},
		},
		{
			name: "literal install argv", pkg: models.Package{Name: literalName, Present: true},
			next: map[string]executil.MockResult{
				"dpkg [-s " + literalName + "]":            {Err: missing},
				"apt-get [install -y " + literalName + "]": {},
			},
			action: (*apt.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "dpkg", Args: []string{"-s", literalName}},
				{Name: "dpkg", Args: []string{"-s", literalName}},
				{Name: "apt-get", Args: []string{"install", "-y", literalName}},
			},
		},
		{
			name: "exact upgrade compare and install", pkg: models.Package{Name: "curl", Present: true, Version: "2.0", AllowUpgrade: &allow},
			next: map[string]executil.MockResult{
				"dpkg-query [-W -f=${Status}\\t${Version} curl]": {Stdout: []byte("install ok installed\t1.0\n")},
				"dpkg [--compare-versions 1.0 gt 2.0]":           {Err: compareFalse},
				"apt-get [install -y curl=2.0]":                  {},
			},
			action: (*apt.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg", Args: []string{"--compare-versions", "1.0", "gt", "2.0"}},
				{Name: "apt-get", Args: []string{"install", "-y", "curl=2.0"}},
			},
		},
		{
			name: "permitted downgrade", pkg: models.Package{Name: "curl", Present: true, Version: "1.0", AllowDowngrade: &allow},
			next: map[string]executil.MockResult{
				"dpkg-query [-W -f=${Status}\\t${Version} curl]":   {Stdout: []byte("install ok installed\t2.0\n")},
				"dpkg [--compare-versions 2.0 gt 1.0]":             {},
				"apt-get [install -y --allow-downgrades curl=1.0]": {},
			},
			action: (*apt.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\\t${Version}", "curl"}},
				{Name: "dpkg", Args: []string{"--compare-versions", "2.0", "gt", "1.0"}},
				{Name: "apt-get", Args: []string{"install", "-y", "--allow-downgrades", "curl=1.0"}},
			},
		},
		{
			name: "remove", pkg: models.Package{Name: "curl", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}, "apt-get [remove -y curl]": {}},
			action: (*apt.Applicator).Apply,
			want:   []executil.MockCall{{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-get", Args: []string{"remove", "-y", "curl"}}},
		},
		{
			name: "purge", pkg: models.Package{Name: "curl", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePurged}},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}, "apt-get [purge -y curl]": {}},
			action: (*apt.Applicator).Apply,
			want:   []executil.MockCall{{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-get", Args: []string{"purge", "-y", "curl"}}},
		},
		{
			name: "remove dependencies", pkg: models.Package{Name: "curl", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, RemoveDependencies: true},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}, "apt-get [remove -y --autoremove curl]": {}},
			action: (*apt.Applicator).Apply,
			want:   []executil.MockCall{{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-get", Args: []string{"remove", "-y", "--autoremove", "curl"}}},
		},
		{
			name: "hold", pkg: models.Package{Name: "curl", Present: true, Hold: &hold},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}, "apt-mark [showhold curl]": {}, "apt-mark [hold curl]": {}},
			action: (*apt.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-mark", Args: []string{"showhold", "curl"}},
				{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-mark", Args: []string{"showhold", "curl"}},
				{Name: "apt-mark", Args: []string{"hold", "curl"}},
			},
		},
		{
			name: "unhold", pkg: models.Package{Name: "curl", Present: true, Hold: &unhold},
			next:   map[string]executil.MockResult{"dpkg [-s curl]": {}, "apt-mark [showhold curl]": {Stdout: []byte("curl\n")}, "apt-mark [unhold curl]": {}},
			action: (*apt.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-mark", Args: []string{"showhold", "curl"}},
				{Name: "dpkg", Args: []string{"-s", "curl"}}, {Name: "apt-mark", Args: []string{"showhold", "curl"}},
				{Name: "apt-mark", Args: []string{"unhold", "curl"}},
			},
		},
		{
			name: "metadata refresh", pkg: models.Package{Name: "curl", Present: true},
			next:   map[string]executil.MockResult{"apt-get [update]": {}},
			action: (*apt.Applicator).RefreshCache,
			want:   []executil.MockCall{{Name: "apt-get", Args: []string{"update"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &executil.MockRunner{Next: tt.next}
			applicator := apt.New(tt.pkg, runner)
			if err := tt.action(applicator, t.Context()); err != nil {
				t.Fatalf("process action failed: %v", err)
			}
			if !slices.EqualFunc(runner.Calls, tt.want, equalMockCall) {
				t.Fatalf("process calls = %#v, want %#v", runner.Calls, tt.want)
			}
			for _, call := range runner.Calls {
				if call.Name == "sh" || call.Name == "bash" {
					t.Fatalf("provider invoked a shell: %+v", call)
				}
				if call.Name == "apt-get" && len(call.Args) > 0 && call.Args[0] != "update" && !slices.Contains(call.Args, "-y") {
					t.Fatalf("APT transaction is not noninteractive: %+v", call)
				}
				for _, forbidden := range []string{"--force-yes", "--allow-unauthenticated"} {
					if slices.Contains(call.Args, forbidden) {
						t.Fatalf("APT argv contains unsafe flag %q: %+v", forbidden, call)
					}
				}
			}
		})
	}

	if _, ok := apt.New(models.Package{Name: "curl", Present: true}, nil).Exec.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("APT default runner = %T, want SanitizedOSRunner", apt.New(models.Package{Name: "curl", Present: true}, nil).Exec)
	}
}

func TestApplicator_BoundsAPTFailureDiagnostics(t *testing.T) {
	const prefix = "controlled apt diagnostic: "
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"dpkg [-s curl]": {Err: errors.New("missing")},
		"apt-get [install -y curl]": {
			Stderr: []byte(prefix + strings.Repeat("x", 4096)), Err: errors.New("exit status 1"),
		},
	}}
	result := apt.New(models.Package{Name: "curl", Present: true}, runner).ApplyResult(t.Context())
	if result.Err == nil || !strings.Contains(result.Err.Error(), prefix) {
		t.Fatalf("ApplyResult() = %+v, want retained bounded diagnostic", result)
	}
	if len(result.Err.Error()) > 1200 {
		t.Fatalf("APT diagnostic length = %d, want bounded output", len(result.Err.Error()))
	}
}

func equalMockCall(left, right executil.MockCall) bool {
	return left.Name == right.Name && slices.Equal(left.Args, right.Args)
}
