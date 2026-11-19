package queryexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/queryexec"
	osqapp "github.com/DavidHoenisch/remotr/internal/applicators/osquery"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type captureRunner struct {
	name   string
	args   []string
	stdout []byte
	stderr []byte
	err    error
}

func (r *captureRunner) RunContext(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.stdout, r.stderr, r.err
}

// OS-OSQ-006 & OS-OSQ-007: queryexec enforces exact argv (--json --ephemeral --disable_database --disable_events and
// -S for osqueryd), maxRows truncation, maxBytes caps, read-only SQL checks,
// and unsupported reporting when osquery runtime is disabled.
func TestExecutor_ExactArgvBoundsAndUnsupported(t *testing.T) {
	t.Run("disabled runtime reports unsupported without invoking binary", func(t *testing.T) {
		runner := &captureRunner{}
		exec := &queryexec.Executor{
			LoadRuntime: func() (osqapp.RuntimeState, error) {
				return osqapp.RuntimeState{Enabled: false, Mode: models.OsqueryModeDisabled}, nil
			},
			Runner: runner,
		}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "run-1",
			Name:  "os-info",
			SQL:   "SELECT version FROM os_version",
		})
		if res.Status != queryexec.StatusUnsupported {
			t.Fatalf("Status = %q, want unsupported", res.Status)
		}
		if runner.name != "" {
			t.Fatalf("runner invoked despite disabled runtime: %q", runner.name)
		}
	})

	t.Run("osqueryi exact argv and maxRows truncation", func(t *testing.T) {
		rows := []map[string]string{
			{"port": "22", "process": "sshd"},
			{"port": "443", "process": "nginx"},
			{"port": "8443", "process": "remotr"},
		}
		raw, _ := json.Marshal(rows)
		runner := &captureRunner{stdout: raw}
		exec := &queryexec.Executor{
			LoadRuntime: func() (osqapp.RuntimeState, error) {
				return osqapp.RuntimeState{
					Enabled:    true,
					Mode:       models.OsqueryModeManaged,
					BinaryPath: "/usr/lib/remotr/osquery/bin/osqueryi",
				}, nil
			},
			Runner: runner,
		}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID:   "run-2",
			Name:    "listening-ports",
			SQL:     "SELECT port, process FROM listening_ports",
			MaxRows: 2,
			Timeout: 10 * time.Second,
		})
		if res.Status != queryexec.StatusSuccess {
			t.Fatalf("Status = %q err=%q, want success", res.Status, res.ErrorSummary)
		}
		if runner.name != "/usr/lib/remotr/osquery/bin/osqueryi" {
			t.Fatalf("binary = %q", runner.name)
		}
		wantArgs := []string{"--json", "--ephemeral", "--disable_database", "--disable_events", "SELECT port, process FROM listening_ports"}
		if strings.Join(runner.args, "|") != strings.Join(wantArgs, "|") {
			t.Fatalf("argv = %v, want %v", runner.args, wantArgs)
		}
		if len(res.Rows) != 2 || res.RowCount != 2 || !res.Truncated {
			t.Fatalf("rows=%d rowCount=%d truncated=%v, want 2/2/true", len(res.Rows), res.RowCount, res.Truncated)
		}
	})

	t.Run("osqueryd uses -S standalone shell flag", func(t *testing.T) {
		runner := &captureRunner{stdout: []byte(`[{"version":"24.04"}]`)}
		exec := &queryexec.Executor{
			LoadRuntime: func() (osqapp.RuntimeState, error) {
				return osqapp.RuntimeState{
					Enabled:        true,
					Mode:           models.OsqueryModeExisting,
					BinaryPath:     "/opt/Elastic/Agent/data/elastic-agent-1/components/osqueryd",
					UseDaemonShell: true,
				}, nil
			},
			Runner: runner,
		}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "run-3",
			Name:  "os-info",
			SQL:   "SELECT version FROM os_version",
		})
		if res.Status != queryexec.StatusSuccess {
			t.Fatalf("Status = %q err=%q", res.Status, res.ErrorSummary)
		}
		wantArgs := []string{"-S", "--json", "--ephemeral", "--disable_database", "--disable_events", "SELECT version FROM os_version"}
		if strings.Join(runner.args, "|") != strings.Join(wantArgs, "|") {
			t.Fatalf("argv = %v, want %v", runner.args, wantArgs)
		}
	})

	t.Run("unsafe SQL rejected before invoking binary", func(t *testing.T) {
		runner := &captureRunner{}
		exec := &queryexec.Executor{
			LoadRuntime: func() (osqapp.RuntimeState, error) {
				return osqapp.RuntimeState{Enabled: true, BinaryPath: "/usr/lib/remotr/osquery/bin/osqueryi"}, nil
			},
			Runner: runner,
		}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "run-4",
			Name:  "bad",
			SQL:   "DROP TABLE users",
		})
		if res.Status != queryexec.StatusFailed || runner.name != "" {
			t.Fatalf("res = %+v runner=%q, want failed without execution", res, runner.name)
		}
	})

	t.Run("execution error is bounded and sanitized", func(t *testing.T) {
		runner := &captureRunner{stderr: []byte(strings.Repeat("x", 1000)), err: errors.New("exit status 1")}
		exec := &queryexec.Executor{
			LoadRuntime: func() (osqapp.RuntimeState, error) {
				return osqapp.RuntimeState{Enabled: true, BinaryPath: "/usr/lib/remotr/osquery/bin/osqueryi"}, nil
			},
			Runner: runner,
		}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "run-5",
			Name:  "err-q",
			SQL:   "SELECT * FROM nonexistent_table",
		})
		if res.Status != queryexec.StatusFailed || len(res.ErrorSummary) == 0 || len(res.ErrorSummary) > 256 {
			t.Fatalf("res = %+v, want bounded error summary <= 256 chars", res)
		}
	})
}
