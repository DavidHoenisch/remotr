package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/queryscheduler"
)

func queryCommand() *cli.Command {
	return &cli.Command{
		Name:     "query",
		Category: catFleet,
		Usage:    "inspect and run fleet osquery queries",
		Description: withExamples(
			`Manage and inspect GitOps-scheduled and ad-hoc osquery results across fleets and endpoints.`,
			"remotr query list --fleet prod",
			"remotr query results listening-ports --fleet prod",
			"remotr query results listening-ports --fleet prod --group-by",
			"remotr query results listening-ports --fleet prod --tui",
			"remotr query diff listening-ports --fleet prod --since 24h",
			"remotr query run listening-ports --endpoint <id>",
		),
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list composed fleet queries, schedules, and reporting coverage",
				Action: actionQueryList,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "fleet", Usage: "filter by fleet name"},
				),
			},
			{
				Name:      "status",
				Usage:     "show per-endpoint execution health for a query",
				ArgsUsage: "<query-name>",
				Action:    actionQueryStatus,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "fleet", Usage: "filter by fleet name"},
					&cli.BoolFlag{Name: "only-errors", Usage: "show only failed or unsupported endpoints"},
				),
			},
			{
				Name:      "results",
				Usage:     "inspect latest snapshot results across a fleet or endpoint",
				ArgsUsage: "<query-name>",
				Action:    actionQueryResults,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "fleet", Usage: "filter by fleet name"},
					&cli.StringFlag{Name: "endpoint", Usage: "filter by endpoint ID"},
					&cli.BoolFlag{Name: "group-by", Aliases: []string{"count"}, Usage: "aggregate identical rows across endpoints to surface outliers"},
					&cli.StringFlag{Name: "sort", Value: "asc", Usage: "group sort order (asc = rarest first, desc = most common first)"},
					&cli.StringSliceFlag{Name: "where", Usage: "filter rows by col=val (repeatable)"},
					&cli.BoolFlag{Name: "non-empty", Usage: "omit endpoints with zero rows"},
					&cli.BoolFlag{Name: "wide", Usage: "disable automatic card fallback for wide tables"},
					&cli.BoolFlag{Name: "tui", Usage: "open results in the interactive TUI viewer"},
					&cli.StringFlag{Name: "save", Usage: "write output to a file path"},
				),
			},
			{
				Name:      "diff",
				Usage:     "show added (+) and removed (-) rows for a diff-mode query",
				ArgsUsage: "<query-name>",
				Action:    actionQueryDiff,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "fleet", Usage: "filter by fleet name"},
					&cli.StringFlag{Name: "endpoint", Usage: "filter by endpoint ID"},
					&cli.StringFlag{Name: "since", Usage: "duration (e.g. 24h) or RFC3339 timestamp"},
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "maximum diff entries to display"},
				),
			},
			{
				Name:      "run",
				Usage:     "trigger an ad-hoc live query and wait for results",
				ArgsUsage: "[query-name]",
				Action:    actionQueryRun,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "fleet", Usage: "target fleet name"},
					&cli.StringFlag{Name: "endpoint", Usage: "target endpoint ID"},
					&cli.StringFlag{Name: "sql", Usage: "inline read-only SQL (requires query_sql_runner/admin role)"},
					&cli.DurationFlag{Name: "timeout", Value: 2 * time.Minute, Usage: "max wait for endpoint responses"},
					&cli.IntFlag{Name: "max-rows", Value: 500, Usage: "row cap per endpoint"},
					&cli.BoolFlag{Name: "group-by", Aliases: []string{"count"}, Usage: "aggregate identical rows across endpoints"},
					&cli.StringFlag{Name: "save", Usage: "write output to a file path"},
				),
			},
		},
	}
}

type flattenedQueryRow struct {
	EndpointID string            `json:"endpoint_id"`
	Truncated  bool              `json:"truncated,omitempty"`
	Values     map[string]string `json:"values"`
}

type groupedQueryRow struct {
	Count           int               `json:"count"`
	Endpoints       []string          `json:"endpoints"`
	SampleEndpoints string            `json:"sample_endpoints"`
	Values          map[string]string `json:"values"`
}

func actionQueryList(ctx context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "query list: %v", err)
	}
	if err := requireOperatorCLI(settings, "query list"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "query list: %v", err)
	}
	queries, err := client.ListQueries(ctx, strings.TrimSpace(c.String("fleet")))
	if err != nil {
		return apiErr(c, "query list", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(queries)
	}
	if len(queries) == 0 {
		fmt.Println("No fleet queries configured.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FLEET\tQUERY\tMODE\tSCHEDULE\tCOVERAGE\tFAILED\tUNSUPPORTED")
	for _, q := range queries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%d\t%d\n",
			q.Fleet, q.Name, q.Mode, q.Schedule, q.Reported, q.TotalHosts, q.Failed, q.Unsupported)
	}
	return tw.Flush()
}

func actionQueryStatus(ctx context.Context, c *cli.Command) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "query status: <query-name> is required")
	}
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "query status: %v", err)
	}
	if err := requireOperatorCLI(settings, "query status"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "query status: %v", err)
	}
	statuses, err := client.GetQueryStatus(ctx, name, strings.TrimSpace(c.String("fleet")))
	if err != nil {
		return apiErr(c, "query status", err)
	}
	if c.Bool("only-errors") {
		filtered := make([]admin.QueryExecutionStatus, 0, len(statuses))
		for _, s := range statuses {
			if s.Status == "failed" || s.Status == "unsupported" {
				filtered = append(filtered, s)
			}
		}
		statuses = filtered
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(statuses)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ENDPOINT\tFLEET\tSTATUS\tROWS\tTRUNCATED\tDURATION\tCOMPLETED\tERROR")
	for _, s := range statuses {
		completed := "-"
		if s.CompletedAt != nil {
			completed = s.CompletedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%t\t%dms\t%s\t%s\n",
			s.EndpointID, s.Fleet, s.Status, s.RowCount, s.Truncated, s.DurationMS, completed, s.ErrorSummary)
	}
	return tw.Flush()
}

func actionQueryResults(ctx context.Context, c *cli.Command) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "query results: <query-name> is required")
	}
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "query results: %v", err)
	}
	if err := requireOperatorCLI(settings, "query results"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "query results: %v", err)
	}
	res, err := client.GetQueryResults(ctx, name, strings.TrimSpace(c.String("fleet")), strings.TrimSpace(c.String("endpoint")))
	if err != nil {
		return apiErr(c, "query results", err)
	}

	opts := queryRenderOptions{
		QueryName: name,
		Fleet:     strings.TrimSpace(c.String("fleet")),
		Format:    strings.ToLower(strings.TrimSpace(c.String("format"))),
		GroupBy:   c.Bool("group-by"),
		SortOrder: strings.ToLower(strings.TrimSpace(c.String("sort"))),
		Where:     c.StringSlice("where"),
		NonEmpty:  c.Bool("non-empty"),
		Wide:      c.Bool("wide"),
		TermWidth: 120,
	}
	if c.Bool("json") {
		opts.Format = "json"
	}

	if c.Bool("tui") {
		flat, cols, _ := flattenQuerySnapshots(res.Snapshots, opts.Where, opts.NonEmpty)
		return runQueryResultsViewer(name, flat, cols)
	}

	var buf bytes.Buffer
	if err := renderQueryResults(&buf, res, opts); err != nil {
		return exitErr(2, "query results: %v", err)
	}
	if savePath := strings.TrimSpace(c.String("save")); savePath != "" {
		if err := os.WriteFile(savePath, buf.Bytes(), 0o600); err != nil {
			return exitErr(1, "query results: save %s: %v", savePath, err)
		}
		fmt.Printf("saved query results to %s\n", savePath)
		return nil
	}
	_, err = io.Copy(os.Stdout, &buf)
	return err
}

func actionQueryDiff(ctx context.Context, c *cli.Command) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return exitErr(2, "query diff: <query-name> is required")
	}
	var since time.Time
	if rawSince := strings.TrimSpace(c.String("since")); rawSince != "" {
		if dur, err := time.ParseDuration(rawSince); err == nil {
			since = time.Now().UTC().Add(-dur)
		} else if parsed, err := time.Parse(time.RFC3339, rawSince); err == nil {
			since = parsed.UTC()
		} else {
			return exitErr(2, "query diff: invalid --since %q (use duration like 24h or RFC3339)", rawSince)
		}
	}
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "query diff: %v", err)
	}
	if err := requireOperatorCLI(settings, "query diff"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "query diff: %v", err)
	}
	diffs, err := client.GetQueryDiff(ctx, name, strings.TrimSpace(c.String("fleet")), strings.TrimSpace(c.String("endpoint")), since, c.Int("limit"))
	if err != nil {
		return apiErr(c, "query diff", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(diffs)
	}
	renderQueryDiffs(os.Stdout, diffs)
	return nil
}

func actionQueryRun(ctx context.Context, c *cli.Command) error {
	name := strings.TrimSpace(c.Args().First())
	sqlText := strings.TrimSpace(c.String("sql"))
	if name == "" && sqlText == "" {
		return exitErr(2, "query run: provide <query-name> or --sql")
	}
	fleet := strings.TrimSpace(c.String("fleet"))
	endpoint := strings.TrimSpace(c.String("endpoint"))
	if (fleet == "" && endpoint == "") || (fleet != "" && endpoint != "") {
		return exitErr(2, "query run: specify exactly one of --fleet or --endpoint")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "query run: %v", err)
	}
	if err := requireOperatorCLI(settings, "query run"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "query run: %v", err)
	}

	created, err := client.RunAdhocQuery(ctx, admin.RunQueryOptions{
		QueryName:  name,
		SQL:        sqlText,
		Fleet:      fleet,
		EndpointID: endpoint,
		MaxRows:    c.Int("max-rows"),
	})
	if err != nil {
		return apiErr(c, "query run", err)
	}

	deadline := time.Now().Add(c.Duration("timeout"))
	current := created
	for time.Now().Before(deadline) {
		if current.ExpectedTargets > 0 && len(current.Results) >= current.ExpectedTargets {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
		polled, pollErr := client.GetAdhocQueryRun(ctx, created.Request.RequestID)
		if pollErr == nil {
			current = polled
		}
	}

	snapshots := make([]admin.QuerySnapshot, 0, len(current.Results))
	statuses := make([]admin.QueryExecutionStatus, 0, len(current.Results))
	for _, r := range current.Results {
		completed := r.CompletedAt
		statuses = append(statuses, admin.QueryExecutionStatus{
			EndpointID:   r.EndpointID,
			Status:       r.Status,
			RowCount:     r.RowCount,
			Truncated:    r.Truncated,
			DurationMS:   r.DurationMS,
			ErrorSummary: r.ErrorSummary,
			CompletedAt:  &completed,
		})
		if r.Status == "success" {
			snapshots = append(snapshots, admin.QuerySnapshot{
				EndpointID: r.EndpointID,
				QueryName:  name,
				Fleet:      fleet,
				Rows:       r.Rows,
				RowCount:   r.RowCount,
				Truncated:  r.Truncated,
				UpdatedAt:  r.CompletedAt,
			})
		}
	}

	opts := queryRenderOptions{
		QueryName: name,
		Fleet:     fleet,
		Format:    strings.ToLower(strings.TrimSpace(c.String("format"))),
		GroupBy:   c.Bool("group-by"),
		SortOrder: "asc",
		TermWidth: 120,
	}
	if c.Bool("json") {
		opts.Format = "json"
	}
	var buf bytes.Buffer
	if err := renderQueryResults(&buf, admin.QueryResultsResponse{
		QueryName: name,
		Fleet:     fleet,
		Endpoint:  endpoint,
		Snapshots: snapshots,
		Statuses:  statuses,
	}, opts); err != nil {
		return exitErr(2, "query run: %v", err)
	}
	if savePath := strings.TrimSpace(c.String("save")); savePath != "" {
		if err := os.WriteFile(savePath, buf.Bytes(), 0o600); err != nil {
			return exitErr(1, "query run: save %s: %v", savePath, err)
		}
		fmt.Printf("saved query run results to %s\n", savePath)
		return nil
	}
	_, err = io.Copy(os.Stdout, &buf)
	return err
}

type queryRenderOptions struct {
	QueryName string
	Fleet     string
	Format    string // "" / table | json | ndjson | csv
	GroupBy   bool
	SortOrder string // asc | desc
	Where     []string
	NonEmpty  bool
	Wide      bool
	TermWidth int
}

func renderQueryResults(w io.Writer, res admin.QueryResultsResponse, opts queryRenderOptions) error {
	flat, cols, truncatedCount := flattenQuerySnapshots(res.Snapshots, opts.Where, opts.NonEmpty)
	format := opts.Format
	if format == "" {
		format = "table"
	}

	switch format {
	case "json":
		payload := map[string]any{
			"query_name":          res.QueryName,
			"fleet":               res.Fleet,
			"endpoint":            res.Endpoint,
			"truncated_endpoints": truncatedCount,
		}
		if opts.GroupBy {
			payload["groups"] = aggregateQueryRows(flat, opts.SortOrder)
		} else {
			payload["rows"] = flat
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)

	case "ndjson":
		enc := json.NewEncoder(w)
		if opts.GroupBy {
			for _, g := range aggregateQueryRows(flat, opts.SortOrder) {
				if err := enc.Encode(g); err != nil {
					return err
				}
			}
			return nil
		}
		for _, row := range flat {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
		return nil

	case "csv":
		cw := csv.NewWriter(w)
		if opts.GroupBy {
			groups := aggregateQueryRows(flat, opts.SortOrder)
			header := append([]string{"COUNT", "SAMPLE_ENDPOINTS"}, cols...)
			if err := cw.Write(header); err != nil {
				return err
			}
			for _, g := range groups {
				rec := make([]string, 0, len(header))
				rec = append(rec, fmt.Sprintf("%d", g.Count), g.SampleEndpoints)
				for _, col := range cols {
					rec = append(rec, g.Values[col])
				}
				if err := cw.Write(rec); err != nil {
					return err
				}
			}
		} else {
			header := append([]string{"ENDPOINT", "TRUNCATED"}, cols...)
			if err := cw.Write(header); err != nil {
				return err
			}
			for _, row := range flat {
				rec := make([]string, 0, len(header))
				rec = append(rec, row.EndpointID, fmt.Sprintf("%t", row.Truncated))
				for _, col := range cols {
					rec = append(rec, row.Values[col])
				}
				if err := cw.Write(rec); err != nil {
					return err
				}
			}
		}
		cw.Flush()
		return cw.Error()

	case "table":
		failed, unsupported := 0, 0
		for _, st := range res.Statuses {
			if st.Status == "failed" {
				failed++
			} else if st.Status == "unsupported" {
				unsupported++
			}
		}
		headerParts := []string{
			fmt.Sprintf("query: %s", res.QueryName),
		}
		if res.Fleet != "" {
			headerParts = append(headerParts, "fleet: "+res.Fleet)
		}
		headerParts = append(headerParts,
			fmt.Sprintf("%d endpoints", len(res.Snapshots)),
			fmt.Sprintf("%d rows", len(flat)),
		)
		if failed > 0 {
			headerParts = append(headerParts, fmt.Sprintf("%d failed", failed))
		}
		if unsupported > 0 {
			headerParts = append(headerParts, fmt.Sprintf("%d unsupported", unsupported))
		}
		if truncatedCount > 0 {
			headerParts = append(headerParts, fmt.Sprintf("WARNING: %d truncated", truncatedCount))
		}
		fmt.Fprintln(w, strings.Join(headerParts, " · "))

		if len(flat) == 0 {
			fmt.Fprintln(w, "(no matching rows)")
			return nil
		}

		if opts.GroupBy {
			groups := aggregateQueryRows(flat, opts.SortOrder)
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "COUNT\tSAMPLE_ENDPOINTS\t%s\n", strings.Join(upperSlice(cols), "\t"))
			for _, g := range groups {
				vals := make([]string, len(cols))
				for i, col := range cols {
					vals[i] = truncateCell(g.Values[col], 48)
				}
				fmt.Fprintf(tw, "%d\t%s\t%s\n", g.Count, g.SampleEndpoints, strings.Join(vals, "\t"))
			}
			return tw.Flush()
		}

		// Automatic vertical key-value card fallback for wide tables.
		termWidth := opts.TermWidth
		if termWidth <= 0 {
			termWidth = 120
		}
		estimatedWidth := 20 + len(cols)*18
		if !opts.Wide && estimatedWidth > termWidth {
			for i, row := range flat {
				if i > 0 {
					fmt.Fprintln(w, "---")
				}
				truncTag := ""
				if row.Truncated {
					truncTag = " [TRUNCATED]"
				}
				fmt.Fprintf(w, "endpoint: %s%s\n", row.EndpointID, truncTag)
				for _, col := range cols {
					fmt.Fprintf(w, "  %s: %s\n", col, row.Values[col])
				}
			}
			return nil
		}

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "ENDPOINT\t%s\n", strings.Join(upperSlice(cols), "\t"))
		for _, row := range flat {
			epLabel := row.EndpointID
			if row.Truncated {
				epLabel += "*"
			}
			vals := make([]string, len(cols))
			for i, col := range cols {
				vals[i] = truncateCell(row.Values[col], 48)
			}
			fmt.Fprintf(tw, "%s\t%s\n", epLabel, strings.Join(vals, "\t"))
		}
		return tw.Flush()

	default:
		return fmt.Errorf("unsupported format %q (want table, json, ndjson, or csv)", opts.Format)
	}
}

func flattenQuerySnapshots(snapshots []admin.QuerySnapshot, where []string, nonEmpty bool) ([]flattenedQueryRow, []string, int) {
	colSet := map[string]struct{}{}
	filters := parseWhereFilters(where)
	var rows []flattenedQueryRow
	truncatedEndpoints := 0

	for _, snap := range snapshots {
		if nonEmpty && len(snap.Rows) == 0 {
			continue
		}
		if snap.Truncated {
			truncatedEndpoints++
		}
		for _, r := range snap.Rows {
			if !matchesWhereFilters(r, filters) {
				continue
			}
			for k := range r {
				colSet[k] = struct{}{}
			}
			rows = append(rows, flattenedQueryRow{
				EndpointID: snap.EndpointID,
				Truncated:  snap.Truncated,
				Values:     r,
			})
		}
	}
	cols := make([]string, 0, len(colSet))
	for c := range colSet {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return rows, cols, truncatedEndpoints
}

func aggregateQueryRows(rows []flattenedQueryRow, sortOrder string) []groupedQueryRow {
	type acc struct {
		endpoints map[string]struct{}
		values    map[string]string
		key       string
	}
	groups := map[string]*acc{}
	for _, r := range rows {
		key := queryscheduler.CanonicalRowKey(r.Values)
		g, ok := groups[key]
		if !ok {
			g = &acc{endpoints: map[string]struct{}{}, values: r.Values, key: key}
			groups[key] = g
		}
		g.endpoints[r.EndpointID] = struct{}{}
	}
	out := make([]groupedQueryRow, 0, len(groups))
	for _, g := range groups {
		eps := make([]string, 0, len(g.endpoints))
		for ep := range g.endpoints {
			eps = append(eps, ep)
		}
		sort.Strings(eps)
		sample := strings.Join(eps, ",")
		if len(eps) > 3 {
			sample = fmt.Sprintf("%s,+%d more", strings.Join(eps[:3], ","), len(eps)-3)
		}
		out = append(out, groupedQueryRow{
			Count:           len(eps),
			Endpoints:       eps,
			SampleEndpoints: sample,
			Values:          g.values,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			if sortOrder == "desc" {
				return out[i].Count > out[j].Count
			}
			return out[i].Count < out[j].Count
		}
		return queryscheduler.CanonicalRowKey(out[i].Values) < queryscheduler.CanonicalRowKey(out[j].Values)
	})
	return out
}

func renderQueryDiffs(w io.Writer, diffs []admin.QueryDiff) {
	if len(diffs) == 0 {
		fmt.Fprintln(w, "No query diffs recorded.")
		return
	}
	for i, d := range diffs {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "endpoint: %s · run: %s · %s (+%d / -%d)\n",
			d.EndpointID, d.RunID, d.CreatedAt.UTC().Format(time.RFC3339), d.AddedCount, d.RemovedCount)
		for _, row := range d.AddedRows {
			fmt.Fprintf(w, "  + %s\n", formatKVLine(row))
		}
		for _, row := range d.RemovedRows {
			fmt.Fprintf(w, "  - %s\n", formatKVLine(row))
		}
	}
}

func formatKVLine(row map[string]string) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, row[k]))
	}
	return strings.Join(parts, " ")
}

func parseWhereFilters(where []string) [][2]string {
	var out [][2]string
	for _, expr := range where {
		parts := strings.SplitN(expr, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
			out = append(out, [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])})
		}
	}
	return out
}

func matchesWhereFilters(row map[string]string, filters [][2]string) bool {
	for _, f := range filters {
		if row[f[0]] != f[1] {
			return false
		}
	}
	return true
}

func upperSlice(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}

func truncateCell(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
