package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v2"
)

func logsCommand() *cli.Command {
	return &cli.Command{
		Name:  "logs",
		Usage: "view server audit log events",
		Subcommands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list audit events from the server",
				Action: actionLogsList,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "since", Usage: "RFC3339 timestamp or duration (e.g. 24h)"},
					&cli.StringFlag{Name: "until", Usage: "RFC3339 timestamp"},
					&cli.StringFlag{Name: "action", Usage: "filter by action (e.g. admin.endpoint.delete)"},
					&cli.StringFlag{Name: "actor-type", Usage: "filter by actor type: operator, endpoint, anonymous"},
					&cli.IntFlag{Name: "limit", Value: 100, Usage: "maximum events per page (max 1000)"},
					&cli.StringFlag{Name: "cursor", Usage: "pagination cursor from a previous page"},
					&cli.BoolFlag{Name: "json", Usage: "output JSON"},
				},
			},
			{
				Name:   "export-info",
				Usage:  "show the secret SIEM export path for this server",
				Action: actionLogsExportInfo,
				Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
		},
	}
}

func actionLogsList(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "logs list: %v", err)
	}
	if err := requireOperatorCLI(settings, "logs list"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "logs list: %v", err)
	}

	since, err := parseAuditTimeFlag(c.String("since"))
	if err != nil {
		return exitErr(2, "logs list: invalid --since: %v", err)
	}
	until, err := parseAuditTimeFlag(c.String("until"))
	if err != nil {
		return exitErr(2, "logs list: invalid --until: %v", err)
	}

	page, err := client.ListAuditEvents(admin.AuditListOptions{
		Since:     since,
		Until:     until,
		Action:    c.String("action"),
		ActorType: c.String("actor-type"),
		Limit:     c.Int("limit"),
		Cursor:    c.String("cursor"),
	})
	if err != nil {
		return exitErr(1, "logs list: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(page)
	}

	if len(page.Events) == 0 {
		fmt.Println("no audit events")
		return nil
	}

	for _, event := range page.Events {
		fmt.Printf("%s\t%s\t%s %s\tstatus=%d\tactor=%s",
			event.OccurredAt.UTC().Format(time.RFC3339),
			event.Action,
			event.Method,
			event.Path,
			event.StatusCode,
			event.ActorType,
		)
		if event.ActorID != "" {
			fmt.Printf(":%s", event.ActorID)
		}
		fmt.Println()
	}
	if page.NextCursor != "" {
		fmt.Printf("\nnext cursor: %s\n", page.NextCursor)
	}
	return nil
}

func actionLogsExportInfo(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "logs export-info: %v", err)
	}
	if err := requireOperatorCLI(settings, "logs export-info"); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "logs export-info: %v", err)
	}

	info, err := client.GetAuditExportInfo()
	if err != nil {
		return exitErr(1, "logs export-info: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Printf("export path: %s\n", info.ExportPath)
	fmt.Printf("path key: %s\n", info.PathKey)
	return nil
}

func parseAuditTimeFlag(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
