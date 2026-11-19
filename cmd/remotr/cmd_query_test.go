package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

// OS-OSQ-011 & OS-OSQ-012: CLI renders flattened fleet tables, wide-result
// vertical cards, --group-by anomaly aggregation, +/- diffs, CSV/JSON/NDJSON
// exports with truncation indicators, and Bubble Tea TUI interactions.
func TestRenderQueryResults_TableCardsGroupByAndExports(t *testing.T) {
	res := admin.QueryResultsResponse{
		QueryName: "listening-ports",
		Fleet:     "prod",
		Snapshots: []admin.QuerySnapshot{
			{
				EndpointID: "ep-1",
				RowCount:   1,
				Rows:       []map[string]string{{"port": "22", "process": "sshd"}},
			},
			{
				EndpointID: "ep-2",
				RowCount:   1,
				Rows:       []map[string]string{{"port": "22", "process": "sshd"}},
			},
			{
				EndpointID: "ep-3",
				RowCount:   2,
				Truncated:  true,
				Rows: []map[string]string{
					{"port": "22", "process": "sshd"},
					{"port": "9999", "process": "rogue-daemon"},
				},
			},
		},
		Statuses: []admin.QueryExecutionStatus{
			{EndpointID: "ep-4", Status: "unsupported"},
		},
	}

	t.Run("default table includes coverage header and truncation warning", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderQueryResults(&buf, res, queryRenderOptions{
			QueryName: "listening-ports",
			Fleet:     "prod",
			TermWidth: 120,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		for _, want := range []string{"query: listening-ports", "3 endpoints", "4 rows", "1 unsupported", "WARNING: 1 truncated", "ep-3*", "rogue-daemon"} {
			if !strings.Contains(out, want) {
				t.Fatalf("table output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("group-by sorts rarest anomaly first by default", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderQueryResults(&buf, res, queryRenderOptions{
			QueryName: "listening-ports",
			Fleet:     "prod",
			GroupBy:   true,
			SortOrder: "asc",
			TermWidth: 120,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		idxRogue := strings.Index(out, "rogue-daemon")
		idxSSHD := strings.Index(out, "sshd")
		if idxRogue < 0 || idxSSHD < 0 || idxRogue > idxSSHD {
			t.Fatalf("expected rare outlier rogue-daemon before common sshd:\n%s", out)
		}
	})

	t.Run("narrow terminal falls back to key-value cards", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderQueryResults(&buf, res, queryRenderOptions{
			QueryName: "listening-ports",
			Fleet:     "prod",
			TermWidth: 40,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "endpoint: ep-3 [TRUNCATED]") {
			t.Fatalf("expected card layout with truncation badge:\n%s", buf.String())
		}
	})

	t.Run("csv export and where filter", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderQueryResults(&buf, res, queryRenderOptions{
			QueryName: "listening-ports",
			Format:    "csv",
			Where:     []string{"port=9999"},
		})
		if err != nil {
			t.Fatal(err)
		}
		recs, err := csv.NewReader(&buf).ReadAll()
		if err != nil || len(recs) != 2 || recs[1][0] != "ep-3" || recs[1][1] != "true" {
			t.Fatalf("csv records = %+v, err=%v", recs, err)
		}
	})

	t.Run("json export includes truncated_endpoints metadata", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderQueryResults(&buf, res, queryRenderOptions{
			QueryName: "listening-ports",
			Format:    "json",
		})
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed["truncated_endpoints"] != float64(1) {
			t.Fatalf("truncated_endpoints = %v, want 1", parsed["truncated_endpoints"])
		}
	})
}

func TestRenderQueryDiffsAndTUIViewer(t *testing.T) {
	var buf bytes.Buffer
	renderQueryDiffs(&buf, []admin.QueryDiff{{
		RunID:        "run-2",
		EndpointID:   "ep-1",
		QueryName:    "listening-ports",
		AddedRows:    []map[string]string{{"port": "443", "process": "nginx"}},
		RemovedRows:  []map[string]string{{"port": "80", "process": "nginx"}},
		AddedCount:   1,
		RemovedCount: 1,
		CreatedAt:    time.Date(2026, 11, 19, 10, 35, 0, 0, time.UTC),
	}})
	out := buf.String()
	if !strings.Contains(out, "+ port=443 process=nginx") || !strings.Contains(out, "- port=80 process=nginx") {
		t.Fatalf("diff output unexpected:\n%s", out)
	}

	// Interactive Bubble Tea viewer toggles grouped view and filters live.
	m := newQueryViewerModel("listening-ports", []flattenedQueryRow{
		{EndpointID: "ep-1", Values: map[string]string{"port": "22", "process": "sshd"}},
		{EndpointID: "ep-2", Values: map[string]string{"port": "9999", "process": "rogue"}},
	}, []string{"port", "process"})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = next.(queryViewerModel)
	if !m.grouped || !strings.Contains(m.View(), "[grouped") {
		t.Fatalf("expected grouped mode after pressing g:\n%s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(queryViewerModel)
	for _, ch := range "rogue" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = next.(queryViewerModel)
	}
	view := m.View()
	if !strings.Contains(view, "rogue") || strings.Contains(view, "sshd") {
		t.Fatalf("expected live filter to isolate rogue:\n%s", view)
	}
}
