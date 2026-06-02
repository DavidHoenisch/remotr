package apt_test

import (
	"context"
	"fmt"
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
			"dpkg [-s nmap]":              {Err: fmt.Errorf("missing")},
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
