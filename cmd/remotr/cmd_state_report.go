package main

import (
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

const staleStateReportWarnAge = 24 * time.Hour
const maxStateReportDiagnostics = 3

func actionEndpointStateReport(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint state report")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint state report: %v", err)
	}
	if err := requireOperatorCLI(settings, "endpoint state report"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "endpoint state report: %v", err)
	}

	report, err := client.GetEndpointStateReport(endpointID)
	if err != nil {
		return apiErr(c, "endpoint state report", err)
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(report); err != nil {
			return exitErr(2, "endpoint state report: %v", err)
		}
	} else {
		printEndpointStateReport(report)
	}

	if report.HasReport() && !report.InCompliance {
		return errDrift()
	}
	return nil
}

func actionFleetStateReport(ctx context.Context, c *cli.Command) error {
	fleet, err := resolveFleet(c, "fleet state report")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "fleet state report: %v", err)
	}
	if err := requireOperatorCLI(settings, "fleet state report"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(2, "fleet state report: %v", err)
	}

	report, err := client.GetFleetStateReport(fleet)
	if err != nil {
		return apiErr(c, "fleet state report", err)
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(report); err != nil {
			return exitErr(2, "fleet state report: %v", err)
		}
	} else {
		printFleetStateReport(report, c.Bool("verbose"))
	}

	if fleetStateHasIssues(report.Summary) {
		return errDrift()
	}
	return nil
}

func printEndpointStateReport(report admin.StateReport) {
	fmt.Printf("endpoint: %s\n", report.EndpointID)
	fmt.Printf("fleet: %s\n", report.Fleet)
	if !report.HasReport() {
		fmt.Println("report: (none)")
		return
	}
	warnStaleStateReport(report.ReportedAt)
	fmt.Println("config:")
	fmt.Printf("  release_ref: %s\n", report.ReleaseRef)
	fmt.Printf("  digest: %s\n", report.Digest)
	fmt.Printf("checked_at: %s\n", report.ReportedAt.UTC().Format(time.RFC3339))
	fmt.Printf("in_compliance: %t\n", report.InCompliance)
	fmt.Printf("status: %s\n", report.Status)
	if len(report.Items) == 0 {
		fmt.Println("resource_items: (none)")
	} else {
		fmt.Println("resource_items:")
		for _, item := range report.Items {
			fmt.Printf("  - address: %s\n", item.Address)
			fmt.Printf("    name: %s\n", item.Name)
			fmt.Printf("    description: %s\n", item.Description)
			fmt.Printf("    provider: %s\n", item.Provider)
			fmt.Printf("    status: %s\n", item.Status)
			fmt.Printf("    reason_code: %s\n", item.ReasonCode)
			if summary := item.DesiredSummary.String(); summary != "" {
				fmt.Printf("    desired_summary: %s\n", summary)
			}
			if summary := item.ObservedSummary.String(); summary != "" {
				fmt.Printf("    observed_summary: %s\n", summary)
			}
			if len(item.Subresults) > 0 {
				fmt.Println("    subresults:")
				for _, subresult := range item.Subresults {
					fmt.Printf("      - target: %s\n", subresult.Target)
					fmt.Printf("        status: %s\n", subresult.Status)
					fmt.Printf("        reason_code: %s\n", subresult.ReasonCode)
					if summary := subresult.ObservedSummary.String(); summary != "" {
						fmt.Printf("        observed_summary: %s\n", summary)
					}
				}
			}
			if item.SubresultsTruncated {
				fmt.Println("    subresults_truncated: true")
			}
		}
	}
	if len(report.ScheduleRuntime) > 0 {
		fmt.Println("schedule_runtime:")
		for _, runtime := range report.ScheduleRuntime {
			fmt.Printf("  - address: %s\n", runtime.Address)
			fmt.Printf("    name: %s\n", runtime.Name)
			fmt.Printf("    provider: %s\n", runtime.Provider)
			fmt.Printf("    status: %s\n", runtime.Status)
			if runtime.ExitCode != nil {
				fmt.Printf("    exit_code: %d\n", *runtime.ExitCode)
			}
			fmt.Printf("    missed_run_behavior: %s\n", runtime.MissedRunBehavior)
		}
	}
	printRebootRequired(report.RebootRequired)
	printApplyResults(report.Apply)
	printApplyFailureSection(report.ApplyFailure)
}

func printRebootRequired(status *admin.StateReportRebootRequired) {
	if status == nil {
		fmt.Println("reboot_required: false")
		return
	}
	fmt.Printf("reboot_required: %t\n", status.Required)
	if status.Required {
		if len(status.Sources) == 0 {
			fmt.Println("reboot_sources: (none)")
		} else {
			fmt.Println("reboot_sources:")
			for _, source := range status.Sources {
				fmt.Printf("  - address: %s\n", source.Address)
				fmt.Printf("    name: %s\n", source.Name)
				fmt.Printf("    provider: %s\n", source.Provider)
			}
		}
	}
	if status.AttemptGeneration > 0 {
		fmt.Printf("reboot_attempt_generation: %d\n", status.AttemptGeneration)
	}
	if status.Intent != nil {
		intent := status.Intent
		fmt.Printf("reboot_generation: %s\n", intent.Generation)
		fmt.Printf("reboot_phase: %s\n", intent.Phase)
		fmt.Printf("reboot_prior_boot_id: %s\n", intent.PriorBootID)
		if intent.CurrentBootID != "" {
			fmt.Printf("reboot_current_boot_id: %s\n", intent.CurrentBootID)
		}
		if intent.Reason != "" {
			fmt.Printf("reboot_reason: %s\n", intent.Reason)
		}
	}
	if status.Completion != nil {
		completion := status.Completion
		fmt.Printf("reboot_completed_generation: %s\n", completion.Generation)
		fmt.Printf("reboot_completed_boot_id: %s\n", completion.BootID)
		fmt.Printf("reboot_completed_attempt_generation: %d\n", completion.AttemptGeneration)
		if !completion.CompletedAt.IsZero() {
			fmt.Printf("reboot_completed_at: %s\n", completion.CompletedAt.UTC().Format(time.RFC3339))
		}
	}
}

func printFleetStateReport(report admin.FleetStateReport, verbose bool) {
	fmt.Printf("%s (%d endpoints)\n", report.Fleet, report.Summary.Total)
	fmt.Printf("  IN COMPLIANCE   %d\n", report.Summary.Compliant)
	fmt.Printf("  DRIFTED         %d\n", report.Summary.Drift)
	fmt.Printf("  UNSUPPORTED     %d\n", report.Summary.Unsupported)
	fmt.Printf("  CHECK FAILED    %d\n", report.Summary.CheckFailed)
	fmt.Printf("  DEFERRED        %d\n", report.Summary.Deferred)
	fmt.Printf("  APPLY FAILED    %d\n", report.Summary.ApplyFailed)
	fmt.Printf("  NO REPORT       %d\n", report.Summary.NoReport)

	if fleetStateHasIssues(report.Summary) {
		fmt.Println()
		fmt.Println("ATTENTION REQUIRED")
		for _, ep := range report.Endpoints {
			if !ep.HasReport() || ep.Status == admin.StateCompliant {
				continue
			}
			warnStaleStateReport(ep.ReportedAt)
			for _, item := range ep.Items {
				fmt.Printf("  %s   %s   [%s]   %s\n", ep.EndpointID, item.Address, item.Status, item.ReasonCode)
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
		printEndpointStateReport(ep)
	}
}

func fleetStateHasIssues(summary admin.FleetStateSummary) bool {
	return summary.Drift > 0 || summary.Unsupported > 0 || summary.CheckFailed > 0 || summary.Deferred > 0 || summary.ApplyFailed > 0
}

func printApplyResults(items []admin.StateReportApplyItem) {
	if len(items) == 0 {
		fmt.Println("apply_results: (none)")
		return
	}
	fmt.Println("apply_results:")
	for _, item := range items {
		fmt.Printf("  - address: %s\n", item.Address)
		fmt.Printf("    provider: %s\n", item.Provider)
		fmt.Printf("    status: %s\n", item.Status)
		fmt.Printf("    reason_code: %s\n", item.ReasonCode)
		fmt.Printf("    rollback_class: %s\n", item.RollbackClass)
		fmt.Printf("    rollback_status: %s\n", item.RollbackStatus)
		for _, diagnostic := range item.Diagnostics[:min(len(item.Diagnostics), maxStateReportDiagnostics)] {
			fmt.Printf("    diagnostic: %s\n", diagnostic.String())
		}
		if len(item.Diagnostics) > maxStateReportDiagnostics {
			fmt.Printf("    diagnostics_omitted: %d\n", len(item.Diagnostics)-maxStateReportDiagnostics)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func printApplyFailureSection(failure *admin.ApplyFailureSummary) {
	if failure == nil {
		fmt.Println("apply_failure: (none)")
		return
	}
	fmt.Println("apply_failure:")
	fmt.Printf("  release_ref: %s\n", failure.ReleaseRef)
	fmt.Printf("  resource_address: %s\n", failure.ResourceAddress)
	fmt.Printf("  failure: %s\n", failure.Failure.Error())
	if !failure.ReportedAt.IsZero() {
		fmt.Printf("  reported_at: %s\n", failure.ReportedAt.UTC().Format(time.RFC3339))
	}
}

func warnStaleStateReport(reportedAt time.Time) {
	if reportedAt.IsZero() {
		return
	}
	age := time.Since(reportedAt.UTC())
	if age <= staleStateReportWarnAge {
		return
	}
	writeInfo("%s state report is %s old (reported_at %s)\n",
		labelWarn(nil, "warning:"),
		age.Truncate(time.Second),
		reportedAt.UTC().Format(time.RFC3339),
	)
}
