package systemd

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_Apply_runsDaemonReloadBeforeStart(t *testing.T) {
	active := true
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"systemctl [is-enabled foo.service]": {Stdout: []byte("disabled\n")},
			"systemctl [is-active foo.service]":  {Stdout: []byte("inactive\n")},
			"systemctl [daemon-reload]":          {},
			"systemctl [start foo.service]":      {},
		},
	}
	a := New(models.SystemdResource{
		Name:   "foo",
		Unit:   "foo.service",
		Active: &active,
	}, mock)

	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(mock.Calls) < 3 {
		t.Fatalf("calls = %+v", mock.Calls)
	}
	reloadIdx, startIdx := -1, -1
	for i, c := range mock.Calls {
		if c.Name == "systemctl" && len(c.Args) == 1 && c.Args[0] == "daemon-reload" {
			reloadIdx = i
		}
		if c.Name == "systemctl" && len(c.Args) == 2 && c.Args[0] == "start" {
			startIdx = i
		}
	}
	if reloadIdx < 0 || startIdx < 0 {
		t.Fatalf("expected daemon-reload and start, calls = %+v", mock.Calls)
	}
	if reloadIdx > startIdx {
		t.Fatalf("daemon-reload must run before start, calls = %+v", mock.Calls)
	}
}

func TestApplicator_Apply_stopsAndDisablesBeforeMasking(t *testing.T) {
	masked, disabled, stopped := true, false, false
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemctl [is-enabled foo.service]":   {Stdout: []byte("enabled\n")},
		"systemctl [is-active foo.service]":    {Stdout: []byte("active\n")},
		"systemctl [daemon-reload]":            {},
		"systemctl [disable foo.service]":      {},
		"systemctl [stop foo.service]":         {},
		"systemctl [reset-failed foo.service]": {},
		"systemctl [mask foo.service]":         {},
	}}
	a := New(models.SystemdResource{Name: "foo", Unit: "foo.service", Masked: &masked, Enabled: &disabled, Active: &stopped}, mock)

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(mock.Calls))
	for _, call := range mock.Calls {
		if len(call.Args) > 0 && call.Args[0] != "is-enabled" && call.Args[0] != "is-active" && call.Args[0] != "daemon-reload" {
			got = append(got, fmt.Sprintf("%s %s", call.Args[0], call.Args[1]))
		}
	}
	want := []string{"disable foo.service", "stop foo.service", "reset-failed foo.service", "mask foo.service"}
	if !slices.Equal(got, want) {
		t.Fatalf("mutation order = %v, want %v", got, want)
	}
}
