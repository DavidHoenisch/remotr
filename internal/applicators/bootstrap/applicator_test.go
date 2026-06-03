package bootstrap_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_State_pathMissing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "clamav", "main.cvd")
	existing := filepath.Join(dir, "present")
	if err := os.WriteFile(existing, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := models.BootstrapResource{
		Name: "clamav-db",
		When: models.BootstrapWhen{PathMissing: missing},
	}

	a := bootstrap.New(r, nil)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift when path missing")
	}

	r.When = models.BootstrapWhen{PathMissing: existing}
	a = bootstrap.New(r, nil)
	_, met = a.State(context.Background())
	if !met {
		t.Fatal("expected met when path present for pathMissing trigger")
	}
}

func TestApplicator_State_pathExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.lock")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone")

	r := models.BootstrapResource{
		Name: "cleanup",
		When: models.BootstrapWhen{PathExists: path},
	}
	a := bootstrap.New(r, nil)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift when path exists")
	}

	r.When = models.BootstrapWhen{PathExists: missing}
	a = bootstrap.New(r, nil)
	_, met = a.State(context.Background())
	if !met {
		t.Fatal("expected met when path absent for pathExists trigger")
	}
}

func TestApplicator_Apply_execStepsInOrder(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "need-bootstrap")
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"true []":        {Err: nil},
			"freshclam []":  {Err: nil},
		},
	}
	r := models.BootstrapResource{
		Name: "clamav-db",
		When: models.BootstrapWhen{PathMissing: trigger},
		Steps: []models.BootstrapStep{
			{Exec: []string{"true"}},
			{Exec: []string{"freshclam"}},
		},
	}
	a := bootstrap.New(r, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("calls = %v", mock.Calls)
	}
	if mock.Calls[0].Name != "true" || mock.Calls[1].Name != "freshclam" {
		t.Fatalf("unexpected order: %v", mock.Calls)
	}
}

func TestApplicator_Apply_systemdSteps(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "missing")
	stop, start := false, true
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"systemctl [stop freshclam.service]":  {Err: nil},
			"freshclam []":                       {Err: nil},
			"systemctl [start freshclam.service]": {Err: nil},
		},
	}
	r := models.BootstrapResource{
		Name: "freshclam-refresh",
		When: models.BootstrapWhen{PathMissing: trigger},
		Steps: []models.BootstrapStep{
			{Systemd: &models.BootstrapSystemdStep{Unit: "freshclam.service", Active: &stop}},
			{Exec: []string{"freshclam"}},
			{Systemd: &models.BootstrapSystemdStep{Unit: "freshclam.service", Active: &start}},
		},
	}
	a := bootstrap.New(r, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := []string{
		"systemctl [stop freshclam.service]",
		"freshclam []",
		"systemctl [start freshclam.service]",
	}
	if len(mock.Calls) != len(want) {
		t.Fatalf("calls = %v", mock.Calls)
	}
	for i, w := range want {
		got := fmt.Sprintf("%s %v", mock.Calls[i].Name, mock.Calls[i].Args)
		if got != w {
			t.Fatalf("call[%d] = %q, want %q", i, got, w)
		}
	}
}

func TestApplicator_Apply_failFast(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "missing")
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"false []": {Err: errors.New("boom")},
		},
	}
	r := models.BootstrapResource{
		Name: "x",
		When: models.BootstrapWhen{PathMissing: trigger},
		Steps: []models.BootstrapStep{
			{Exec: []string{"false"}},
			{Exec: []string{"true"}},
		},
	}
	a := bootstrap.New(r, mock)
	err := a.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected fail-fast after first step, calls = %v", mock.Calls)
	}
}

func TestApplicator_Apply_alreadyMet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "done")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := models.BootstrapResource{
		Name: "done",
		When: models.BootstrapWhen{PathMissing: path},
		Steps: []models.BootstrapStep{
			{Exec: []string{"true"}},
		},
	}
	a := bootstrap.New(r, &executil.MockRunner{Next: map[string]executil.MockResult{}})
	err := a.Apply(context.Background())
	if !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("Apply() = %v, want ErrStateAlreadyMet", err)
	}
}

func TestApplicator_Apply_systemdEnableDisable(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "missing")
	enabled, disabled := true, false
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"systemctl [enable foo.service]":  {Err: nil},
			"systemctl [disable bar.service]": {Err: nil},
		},
	}
	r := models.BootstrapResource{
		Name: "units",
		When: models.BootstrapWhen{PathMissing: trigger},
		Steps: []models.BootstrapStep{
			{Systemd: &models.BootstrapSystemdStep{Unit: "foo.service", Enabled: &enabled}},
			{Systemd: &models.BootstrapSystemdStep{Unit: "bar.service", Enabled: &disabled}},
		},
	}
	a := bootstrap.New(r, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
}

func TestApplicator_Revert_noOp(t *testing.T) {
	a := bootstrap.New(models.BootstrapResource{Name: "x"}, nil)
	if err := a.Revert(context.Background()); !errors.Is(err, appErr.ErrNoOp) {
		t.Fatalf("Revert() = %v", err)
	}
}

func TestApplicator_IdempotentAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "main.cvd")
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"freshclam []": {Err: nil},
		},
	}
	r := models.BootstrapResource{
		Name: "clamav-db",
		When: models.BootstrapWhen{PathMissing: db},
		Steps: []models.BootstrapStep{
			{Exec: []string{"freshclam"}},
		},
	}
	a := bootstrap.New(r, mock)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("first Apply() = %v", err)
	}
	if err := os.WriteFile(db, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected met after bootstrap created file")
	}
	err := a.Apply(context.Background())
	if !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v", err)
	}
}
