package apt_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_checkInstalled(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"dpkg [-s curl]": {Err: nil},
		},
	}
	a := apt.New(models.Package{Name: "curl", Present: true}, mock)
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected curl present")
	}
}

func TestApplicator_driftWhenAbsent(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"dpkg [-s nmap]": {Err: fmt.Errorf("not installed")},
		},
	}
	a := apt.New(models.Package{Name: "nmap", Present: true}, mock)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift")
	}
}

func TestApplicator_applyInstall(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"dpkg [-s nmap]":            {Err: fmt.Errorf("missing")},
			"apt-get [install -y nmap]": {Err: nil},
		},
	}
	a := apt.New(models.Package{Name: "nmap", Present: true}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(mock.Calls) < 2 {
		t.Fatalf("expected install call, got %v", mock.Calls)
	}
}

func TestApplicator_applyInstallUsesSafeExactArgv(t *testing.T) {
	const packageName = "curl; echo should-not-run"
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"dpkg [-s curl; echo should-not-run]":            {Err: fmt.Errorf("missing")},
			"apt-get [install -y curl; echo should-not-run]": {Err: nil},
		},
	}
	a := apt.New(models.Package{Name: packageName, Present: true}, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	want := []string{"install", "-y", packageName}
	for _, call := range mock.Calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("unexpected shell call: %+v", call)
		}
		if call.Name != "apt-get" {
			continue
		}
		if !slices.Equal(call.Args, want) {
			t.Fatalf("apt-get argv = %#v, want %#v", call.Args, want)
		}
		for _, forbidden := range []string{"--force-yes", "--allow-unauthenticated", "--allow-downgrades"} {
			if slices.Contains(call.Args, forbidden) {
				t.Fatalf("apt-get argv contains forbidden flag %q: %#v", forbidden, call.Args)
			}
		}
		return
	}
	t.Fatalf("apt-get was not called: %+v", mock.Calls)
}
