package pacman_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_PacmanProcessBoundary(t *testing.T) {
	missing := errors.New("package is not installed")
	allow := true
	tests := []struct {
		name   string
		pkg    models.Package
		next   map[string]executil.MockResult
		action func(*pacman.Applicator, context.Context) error
		want   []executil.MockCall
	}{
		{
			name: "installed version query", pkg: models.Package{Name: "curl", Present: true},
			next:   map[string]executil.MockResult{"pacman [-Q curl]": {Stdout: []byte("curl 1.0\n")}},
			action: func(a *pacman.Applicator, ctx context.Context) error { _, _ = a.State(ctx); return nil },
			want:   []executil.MockCall{{Name: "pacman", Args: []string{"-Q", "curl"}}},
		},
		{
			name: "unversioned install", pkg: models.Package{Name: "curl", Present: true},
			next: map[string]executil.MockResult{
				"pacman [-Q curl]": {Err: missing}, "pacman [-S --noconfirm curl]": {},
			},
			action: (*pacman.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "pacman", Args: []string{"-S", "--noconfirm", "curl"}},
			},
		},
		{
			name: "metadata refresh before install", pkg: models.Package{Name: "curl", Present: true, RefreshCache: true},
			next: map[string]executil.MockResult{
				"pacman [-Q curl]": {Err: missing}, "pacman [-Sy --noconfirm]": {}, "pacman [-S --noconfirm curl]": {},
			},
			action: (*pacman.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "pacman", Args: []string{"-Sy", "--noconfirm"}},
				{Name: "pacman", Args: []string{"-S", "--noconfirm", "curl"}},
			},
		},
		{
			name: "exact resolved artifact", pkg: models.Package{Name: "curl", Present: true, Version: "2.0", AllowUpgrade: &allow},
			next: map[string]executil.MockResult{
				"pacman [-Q curl]":                            {Stdout: []byte("curl 1.0\n")},
				"pacman [-Si curl]":                           {Stdout: []byte("Name : curl\nVersion : 2.0\n")},
				"pacman [-Sp --print-format %n\t%v\t%l curl]": {Stdout: []byte("curl\t2.0\tfile:///repo/curl-2.0-x86_64.pkg.tar.zst\n")},
				"vercmp [1.0 2.0]":                            {Stdout: []byte("-1\n")},
				"pacman [-U --noconfirm file:///repo/curl-2.0-x86_64.pkg.tar.zst]": {},
			},
			action: (*pacman.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "pacman", Args: []string{"-Si", "curl"}},
				{Name: "pacman", Args: []string{"-Sp", "--print-format", "%n\t%v\t%l", "curl"}},
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "vercmp", Args: []string{"1.0", "2.0"}},
				{Name: "pacman", Args: []string{"-U", "--noconfirm", "file:///repo/curl-2.0-x86_64.pkg.tar.zst"}},
			},
		},
		{
			name: "remove", pkg: models.Package{Name: "curl", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}},
			next:   map[string]executil.MockResult{"pacman [-Q curl]": {Stdout: []byte("curl 1.0\n")}, "pacman [-R --noconfirm curl]": {}},
			action: (*pacman.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "pacman", Args: []string{"-R", "--noconfirm", "curl"}},
			},
		},
		{
			name: "remove dependencies", pkg: models.Package{
				Name: "curl", RemoveDependencies: true, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
			},
			next:   map[string]executil.MockResult{"pacman [-Q curl]": {Stdout: []byte("curl 1.0\n")}, "pacman [-Rs --noconfirm curl]": {}},
			action: (*pacman.Applicator).Apply,
			want: []executil.MockCall{
				{Name: "pacman", Args: []string{"-Q", "curl"}},
				{Name: "pacman", Args: []string{"-Rs", "--noconfirm", "curl"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &executil.MockRunner{Next: tt.next}
			provider := pacman.New(tt.pkg, runner)
			if err := tt.action(provider, t.Context()); err != nil {
				t.Fatalf("process action failed: %v", err)
			}
			if !slices.EqualFunc(runner.Calls, tt.want, equalPacmanCall) {
				t.Fatalf("process calls = %#v, want %#v", runner.Calls, tt.want)
			}
			assertPacmanCommandPolicy(t, runner.Calls)
		})
	}

	if _, ok := pacman.New(models.Package{Name: "curl", Present: true}, nil).Exec.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("Pacman default runner = %T, want SanitizedOSRunner", pacman.New(models.Package{Name: "curl", Present: true}, nil).Exec)
	}
}

func TestApplicator_CanceledContextDoesNotCrossPacmanBoundary(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman [-Q curl]": {Err: errors.New("missing")}, "pacman [-S --noconfirm curl]": {},
	}}
	provider := pacman.New(models.Package{Name: "curl", Present: true}, runner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("canceled ApplyResult() = %+v, want context cancellation", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("canceled Apply crossed native process boundary: %+v", runner.Calls)
	}
}

func TestApplicator_BoundsPacmanFailureDiagnostics(t *testing.T) {
	const prefix = "controlled native diagnostic: "
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman [-Q curl]": {Err: errors.New("missing")},
		"pacman [-S --noconfirm curl]": {
			Stderr: []byte(prefix + strings.Repeat("x", 4096)), Err: errors.New("exit status 1"),
		},
	}}
	result := pacman.New(models.Package{Name: "curl", Present: true}, runner).ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), prefix) {
		t.Fatalf("failed ApplyResult() = %+v, want retained bounded diagnostic", result)
	}
	if len(result.Err.Error()) > 1200 {
		t.Fatalf("provider diagnostic length = %d, want bounded output", len(result.Err.Error()))
	}
}

func TestApplicator_ReportsActivationWithoutExecutingIt(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman [-Q telemetry-agent]":             {Err: errors.New("missing")},
		"pacman [-S --noconfirm telemetry-agent]": {},
	}}
	provider := pacman.New(models.Package{
		Name: "telemetry-agent", Present: true,
		ResourceMeta: models.ResourceMeta{Notifications: []models.Notification{{Type: models.NotificationTryRestart, Target: "telemetry.service"}}},
	}, runner)
	result := provider.ApplyResult(t.Context())
	want := []executor.ActivationSignal{{Kind: executor.ActivationTryRestart, Target: "telemetry.service"}}
	if result.Status != executor.Changed || !slices.Equal(result.Activation, want) {
		t.Fatalf("ApplyResult() = %+v, want observable activation %v", result, want)
	}
	if len(runner.Calls) != 2 {
		t.Fatalf("package provider implicitly executed activation: %+v", runner.Calls)
	}
}

func equalPacmanCall(left, right executil.MockCall) bool {
	return left.Name == right.Name && slices.Equal(left.Args, right.Args)
}

func assertPacmanCommandPolicy(t *testing.T, calls []executil.MockCall) {
	t.Helper()
	for _, call := range calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("provider invoked a shell: %+v", call)
		}
		if call.Name != "pacman" || len(call.Args) == 0 {
			continue
		}
		switch call.Args[0] {
		case "-S", "-Sy", "-U", "-R", "-Rs":
			if !slices.Contains(call.Args, "--noconfirm") {
				t.Fatalf("Pacman transaction is interactive: %+v", call)
			}
		}
	}
}
