package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/DavidHoenisch/remotr/internal/admin"
	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
)

func diagnosticsCommand() *cli.Command {
	return &cli.Command{
		Name:     "diagnostics",
		Category: catFleet,
		Usage:    "collect endpoint diagnostic bundles",
		Commands: []*cli.Command{
			{
				Name:      "collect",
				Usage:     "request and wait for a diagnostic bundle from an endpoint",
				ArgsUsage: "[endpoint-id]",
				Action:    actionDiagnosticsCollect,
				Flags: append(outputFlags(),
					endpointIDFlag(),
					&cli.StringFlag{Name: "since", Usage: "RFC3339 start of log range (default: 24h ago)"},
					&cli.StringFlag{Name: "until", Usage: "RFC3339 end of log range (default: now)"},
					&cli.StringSliceFlag{Name: "collectors", Usage: "allowlisted collector IDs (default: all)"},
					&cli.DurationFlag{Name: "timeout", Usage: "max wait for agent collection", Value: 5 * time.Minute},
					&cli.BoolFlag{Name: "stdout", Usage: "write tar.gz bundle to stdout"},
					&cli.StringFlag{Name: "save", Usage: "write tar.gz bundle to path"},
				),
			},
		},
	}
}

func actionDiagnosticsCollect(ctx context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "diagnostics collect")
	if err != nil {
		return err
	}
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "diagnostics collect: %v", err)
	}
	if err := requireOperatorCLI(settings, "diagnostics collect"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "diagnostics collect: %v", err)
	}

	since, until, err := parseDiagnosticTimeRange(c)
	if err != nil {
		return exitErr(2, "diagnostics collect: %v", err)
	}
	collectors := c.StringSlice("collectors")

	var created admin.DiagnosticRequest
	err = runWithSpinner(ctx, c, "requesting diagnostics", func(context.Context) error {
		var reqErr error
		created, reqErr = client.RequestDiagnosticsCollect(endpointID, admin.CollectDiagnosticsOptions{
			Collectors: collectors,
			Since:      since,
			Until:      until,
		})
		return reqErr
	})
	if err != nil {
		return apiErr(c, "diagnostics collect", err)
	}

	timeout := c.Duration("timeout")
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	var final admin.DiagnosticRequest
	err = runWithSpinner(ctx, c, "waiting for endpoint diagnostics", func(spinCtx context.Context) error {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if spinCtx.Err() != nil {
				return spinCtx.Err()
			}
			status, err := client.GetDiagnosticRequest(created.ID)
			if err != nil {
				return err
			}
			switch status.Status {
			case diagcatalog.StatusReady, diagcatalog.StatusFailed, diagcatalog.StatusExpired:
				final = status
				return nil
			}
			time.Sleep(750 * time.Millisecond)
		}
		return fmt.Errorf("timed out after %s waiting for diagnostics", timeout)
	})
	if err != nil {
		return apiErr(c, "diagnostics collect", err)
	}
	if final.Status == diagcatalog.StatusFailed {
		msg := strings.TrimSpace(final.ErrorMessage)
		if msg == "" {
			msg = "diagnostic collection failed"
		}
		return exitErr(1, "diagnostics collect: %s", msg)
	}
	if final.Status == diagcatalog.StatusExpired {
		return exitErr(1, "diagnostics collect: request expired before completion")
	}

	var bundle []byte
	err = runWithSpinner(ctx, c, "downloading diagnostic bundle", func(context.Context) error {
		var dlErr error
		bundle, dlErr = client.DownloadDiagnosticBundle(final.ID)
		return dlErr
	})
	if err != nil {
		return apiErr(c, "diagnostics collect", err)
	}

	if path := strings.TrimSpace(c.String("save")); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return exitErr(1, "diagnostics collect: %v", err)
		}
		if err := os.WriteFile(path, bundle, 0o600); err != nil {
			return exitErr(1, "diagnostics collect: %v", err)
		}
		fmt.Printf("saved diagnostics bundle to %s (%d bytes)\n", path, len(bundle))
		return nil
	}
	if c.Bool("stdout") {
		if _, err := os.Stdout.Write(bundle); err != nil {
			return exitErr(1, "diagnostics collect: %v", err)
		}
		return nil
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(map[string]any{
			"request_id": final.ID,
			"endpoint_id": final.EndpointID,
			"status": final.Status,
			"sha256": final.SHA256,
			"size_bytes": len(bundle),
		})
	}
	if isInteractive() && isStderrTerminal() {
		return runDiagnosticsViewer(bundle)
	}
	fmt.Printf("diagnostics ready: request=%s endpoint=%s bytes=%d sha256=%s\n",
		final.ID, final.EndpointID, len(bundle), final.SHA256)
	fmt.Println("use --stdout or --save to export the bundle")
	return nil
}

func parseDiagnosticTimeRange(c *cli.Command) (time.Time, time.Time, error) {
	var since, until time.Time
	if raw := strings.TrimSpace(c.String("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --since: %w", err)
		}
		since = t
	}
	if raw := strings.TrimSpace(c.String("until")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --until: %w", err)
		}
		until = t
	}
	return since, until, nil
}
