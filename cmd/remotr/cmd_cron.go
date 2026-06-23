package main

import (
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

func actionEndpointCronReport(_ context.Context, c *cli.Command) error {
	endpointID, ok := endpointIDFromFlagOrArg(c)
	if !ok {
		return exitErr(2, "endpoint cron report: endpoint id required (--endpoint or positional)")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint cron report: %v", err)
	}
	if err := requireOperatorCLI(settings, "endpoint cron report"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "endpoint cron report: %v", err)
	}

	report, err := client.GetEndpointCronReport(endpointID)
	if err != nil {
		return apiErr(c, "endpoint cron report", err)
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(report); err != nil {
			return exitErr(2, "endpoint cron report: %v", err)
		}
	} else {
		printEndpointCronReport(report)
	}
	if hasFailedCronJobs(report.Jobs) {
		return exitErr(1, "")
	}
	return nil
}

func actionFleetCronReport(_ context.Context, c *cli.Command) error {
	fleet, ok := fleetFromFlagOrArg(c)
	if !ok {
		return exitErr(2, "fleet cron report: fleet name required (--fleet or positional)")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "fleet cron report: %v", err)
	}
	if err := requireOperatorCLI(settings, "fleet cron report"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "fleet cron report: %v", err)
	}

	report, err := client.GetFleetCronReport(fleet)
	if err != nil {
		return apiErr(c, "fleet cron report", err)
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(report); err != nil {
			return exitErr(2, "fleet cron report: %v", err)
		}
	} else {
		printFleetCronReport(report, c.Bool("verbose"))
	}
	if report.Summary.Failed > 0 {
		return exitErr(1, "")
	}
	return nil
}

func printEndpointCronReport(report admin.CronReport) {
	fmt.Printf("endpoint: %s\n", report.EndpointID)
	fmt.Printf("fleet: %s\n", report.Fleet)
	if report.CronsDigest != "" {
		fmt.Printf("crons_digest: %s\n", report.CronsDigest)
	}
	if len(report.Jobs) == 0 {
		fmt.Println("jobs: (none)")
		return
	}
	fmt.Println("jobs:")
	for _, job := range report.Jobs {
		fmt.Printf("  - name: %s\n", job.Name)
		if job.Schedule != "" {
			fmt.Printf("    schedule: %s\n", job.Schedule)
		}
		fmt.Printf("    applicable: %t\n", job.Applicable)
		if job.LastStatus != "" {
			fmt.Printf("    last_status: %s\n", job.LastStatus)
		}
		if !job.LastScheduledFor.IsZero() {
			fmt.Printf("    last_scheduled_for: %s\n", job.LastScheduledFor.UTC().Format(time.RFC3339))
		}
		if !job.LastCompletedAt.IsZero() {
			fmt.Printf("    last_completed_at: %s\n", job.LastCompletedAt.UTC().Format(time.RFC3339))
		}
		if job.LastMessage != "" {
			fmt.Printf("    last_message: %s\n", job.LastMessage)
		}
	}
}

func printFleetCronReport(report admin.FleetCronReport, verbose bool) {
	fmt.Printf("%s (%d endpoints)\n", report.Fleet, report.Summary.Total)
	fmt.Printf("  SUCCESS   %d\n", report.Summary.Success)
	fmt.Printf("  FAILED    %d\n", report.Summary.Failed)
	fmt.Printf("  RUNNING   %d\n", report.Summary.Running)
	fmt.Printf("  NEVER RUN %d\n", report.Summary.NeverRun)

	if report.Summary.Failed > 0 {
		fmt.Println()
		fmt.Println("FAILED")
		for _, ep := range report.Endpoints {
			for _, job := range ep.Jobs {
				if !job.Applicable || job.LastStatus != "failed" {
					continue
				}
				fmt.Printf("  %s   %s   %s\n", ep.EndpointID, job.Name, job.LastMessage)
			}
		}
	}

	if !verbose {
		return
	}

	fmt.Println()
	fmt.Println("ENDPOINTS")
	for _, ep := range report.Endpoints {
		fmt.Println()
		printEndpointCronReport(ep)
	}
}

func hasFailedCronJobs(jobs []admin.CronJobStatus) bool {
	for _, job := range jobs {
		if job.Applicable && job.LastStatus == "failed" {
			return true
		}
	}
	return false
}
