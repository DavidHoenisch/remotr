// Package queryexec executes bounded read-only osquery queries on an endpoint
// using only the binary authorized by the persisted osquery runtime policy.
package queryexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	osqapp "github.com/DavidHoenisch/remotr/internal/applicators/osquery"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	StatusSuccess     = "success"
	StatusFailed      = "failed"
	StatusUnsupported = "unsupported"

	maxErrorSummaryBytes = 240
)

// CommandRunner is the process execution seam for osqueryi/osqueryd.
type CommandRunner interface {
	RunContext(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

type osCommandRunner struct{}

func (osCommandRunner) RunContext(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- verified osquery binary path and exact argv.
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Request describes one scheduled or ad-hoc query execution dispatched by the server.
type Request struct {
	RunID       string        `json:"run_id"`
	RequestID   string        `json:"request_id,omitempty"`
	TriggerType string        `json:"trigger_type,omitempty"` // scheduled | adhoc
	Name        string        `json:"name"`
	SQL         string        `json:"sql"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	MaxRows     int           `json:"max_rows,omitempty"`
	MaxBytes    int           `json:"max_bytes,omitempty"`
}

// Result is the bounded execution outcome reported back to remotr-server.
type Result struct {
	RunID        string              `json:"run_id"`
	RequestID    string              `json:"request_id,omitempty"`
	TriggerType  string              `json:"trigger_type,omitempty"`
	Name         string              `json:"name"`
	Status       string              `json:"status"`
	Rows         []map[string]string `json:"rows,omitempty"`
	RowCount     int                 `json:"row_count"`
	Truncated    bool                `json:"truncated,omitempty"`
	DurationMS   int64               `json:"duration_ms"`
	ErrorSummary string              `json:"error_summary,omitempty"`
	CompletedAt  time.Time           `json:"completed_at"`
}

// Executor runs read-only osquery queries against the persisted runtime state.
type Executor struct {
	RuntimeStatePath string
	LoadRuntime      func() (osqapp.RuntimeState, error)
	Runner           CommandRunner
	Now              func() time.Time
}

// Execute validates the SQL, checks runtime eligibility, runs osqueryi/osqueryd
// with exact argv, and applies row and payload byte caps.
func (e *Executor) Execute(ctx context.Context, req Request) Result {
	nowFn := e.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	started := nowFn().UTC()
	out := Result{
		RunID:       req.RunID,
		RequestID:   req.RequestID,
		TriggerType: req.TriggerType,
		Name:        req.Name,
	}

	if err := models.ValidateReadOnlySQL(req.SQL); err != nil {
		out.Status = StatusFailed
		out.ErrorSummary = sanitizeError("invalid read-only sql: " + err.Error())
		out.CompletedAt = nowFn().UTC()
		out.DurationMS = out.CompletedAt.Sub(started).Milliseconds()
		return out
	}

	load := e.LoadRuntime
	if load == nil {
		path := e.RuntimeStatePath
		load = func() (osqapp.RuntimeState, error) {
			return osqapp.LoadRuntimeState(path)
		}
	}
	rt, err := load()
	if err != nil {
		out.Status = StatusFailed
		out.ErrorSummary = sanitizeError("load osquery runtime: " + err.Error())
		out.CompletedAt = nowFn().UTC()
		out.DurationMS = out.CompletedAt.Sub(started).Milliseconds()
		return out
	}
	if !rt.Enabled || strings.TrimSpace(rt.BinaryPath) == "" {
		out.Status = StatusUnsupported
		out.ErrorSummary = "osquery runtime is disabled or unavailable"
		out.CompletedAt = nowFn().UTC()
		out.DurationMS = out.CompletedAt.Sub(started).Milliseconds()
		return out
	}

	timeout := req.Timeout
	if timeout <= 0 || timeout > models.MaxQueryTimeout {
		timeout = models.DefaultQueryTimeout
	}
	maxRows := req.MaxRows
	if maxRows <= 0 || maxRows > models.MaxQueryMaxRows {
		maxRows = models.DefaultQueryMaxRows
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > models.DefaultQueryMaxBytes {
		maxBytes = models.DefaultQueryMaxBytes
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, 6)
	if rt.UseDaemonShell {
		args = append(args, "-S")
	}
	args = append(args, "--json", "--ephemeral", "--disable_database", "--disable_events", strings.TrimSpace(req.SQL))

	runner := e.Runner
	if runner == nil {
		runner = osCommandRunner{}
	}
	stdout, stderr, runErr := runner.RunContext(runCtx, rt.BinaryPath, args...)
	out.CompletedAt = nowFn().UTC()
	out.DurationMS = out.CompletedAt.Sub(started).Milliseconds()
	if runErr != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = runErr.Error()
		}
		out.Status = StatusFailed
		out.ErrorSummary = sanitizeError(msg)
		return out
	}

	rows, truncated, parseErr := parseBoundedRows(stdout, maxRows, maxBytes)
	if parseErr != nil {
		out.Status = StatusFailed
		out.ErrorSummary = sanitizeError(parseErr.Error())
		return out
	}
	out.Status = StatusSuccess
	out.Rows = rows
	out.RowCount = len(rows)
	out.Truncated = truncated
	return out
}

func parseBoundedRows(raw []byte, maxRows, maxBytes int) ([]map[string]string, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, nil
	}
	var all []map[string]string
	if err := json.Unmarshal(trimmed, &all); err != nil {
		return nil, false, fmt.Errorf("parse osquery json output: %w", err)
	}
	truncated := false
	if len(all) > maxRows {
		all = all[:maxRows]
		truncated = true
	}
	kept := make([]map[string]string, 0, len(all))
	totalBytes := 2 // "[]"
	for _, row := range all {
		encoded, err := json.Marshal(row)
		if err != nil {
			continue
		}
		if totalBytes+len(encoded)+1 > maxBytes {
			truncated = true
			break
		}
		totalBytes += len(encoded) + 1
		kept = append(kept, row)
	}
	return kept, truncated, nil
}

func sanitizeError(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxErrorSummaryBytes {
		return s[:maxErrorSummaryBytes]
	}
	return s
}
