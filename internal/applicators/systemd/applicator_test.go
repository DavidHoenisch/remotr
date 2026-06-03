package systemd

import (
	"context"
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
