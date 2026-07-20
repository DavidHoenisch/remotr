package pacman_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_exactPacmanVersionConvergence(t *testing.T) {
	allow := true
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman [-Q curl]":                            {Stdout: []byte("curl 1.0\n")},
		"pacman [-Si curl]":                           {Stdout: []byte("Name : curl\nVersion : 2.0\n")},
		"pacman [-Sp --print-format %n\t%v\t%l curl]": {Stdout: []byte("curl\t2.0\tfile:///repo/curl-2.0-x86_64.pkg.tar.zst\n")},
		"vercmp [1.0 2.0]":                            {Stdout: []byte("-1\n")},
		"pacman [-U --noconfirm file:///repo/curl-2.0-x86_64.pkg.tar.zst]": {},
	}}
	a := pacman.New(models.Package{Name: "curl", Present: true, Version: "2.0", AllowUpgrade: &allow}, mock)
	state, met := a.State(context.Background())
	if met || state != "1.0" {
		t.Fatalf("State() = (%v, %t), want (1.0, false)", state, met)
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
}

func TestApplicator_rejectsUnavailablePacmanVersion(t *testing.T) {
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman [-Q curl]":  {Err: fmt.Errorf("missing")},
		"pacman [-Si curl]": {Stdout: []byte("Name : curl\nVersion : 3.0\n")},
	}}
	a := pacman.New(models.Package{Name: "curl", Present: true, Version: "2.0"}, mock)
	if err := a.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Apply() = %v, want unavailable version error", err)
	}
}
