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

const staleStateReportWarnAge = 24 * time.Hour

func actionEndpointStateReport(c *cli.Context) error {
	endpointID := strings.TrimSpace(c.Args().First())
	if endpointID == "" {
		return exitErr(2, "endpoint state report: endpoint id required")
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
		return exitErr(2, "endpoint state report: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return exitErr(2, "endpoint state report: %v", err)
		}
	} else {
		printEndpointStateReport(report)
	}

	if report.HasReport() && !report.InCompliance {
		return exitErr(1, "")
	}
	return nil
}

func actionFleetStateReport(c *cli.Context) error {
	fleet := strings.TrimSpace(c.String("fleet"))
	if fleet == "" {
		return exitErr(2, "fleet state report: --fleet is required")
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
		return exitErr(2, "fleet state report: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return exitErr(2, "fleet state report: %v", err)
		}
	} else {
		printFleetStateReport(report, c.Bool("verbose"))
	}

	if report.Summary.Drift > 0 {
		return exitErr(1, "")
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
	if len(report.Items) == 0 {
		fmt.Println("drift_items: (none)")
	} else {
		fmt.Println("drift_items:")
		for _, item := range report.Items {
			fmt.Printf("  - address: %s\n", item.Address)
			fmt.Printf("    name: %s\n", item.Name)
			fmt.Printf("    description: %s\n", item.Description)
		}
	}
	printApplyFailureSection(report.ApplyFailure)
}

func printFleetStateReport(report admin.FleetStateReport, verbose bool) {
	fmt.Printf("%s (%d endpoints)\n", report.Fleet, report.Summary.Total)
	fmt.Printf("  IN COMPLIANCE   %d\n", report.Summary.Compliant)
	fmt.Printf("  DRIFT           %d\n", report.Summary.Drift)
	fmt.Printf("  NO REPORT       %d\n", report.Summary.NoReport)

	if report.Summary.Drift > 0 {
		fmt.Println()
		fmt.Println("DRIFT")
		for _, ep := range report.Endpoints {
			if !ep.HasReport() || ep.InCompliance {
				continue
			}
			warnStaleStateReport(ep.ReportedAt)
			for _, item := range ep.Items {
				fmt.Printf("  %s   %s   %s\n", ep.EndpointID, item.Address, item.Description)
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

func printApplyFailureSection(failure *admin.ApplyFailureSummary) {
	if failure == nil {
		fmt.Println("apply_failure: (none)")
		return
	}
	fmt.Println("apply_failure:")
	fmt.Printf("  release_ref: %s\n", failure.ReleaseRef)
	fmt.Printf("  resource_address: %s\n", failure.ResourceAddress)
	fmt.Printf("  message: %s\n", failure.Message)
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
	fmt.Fprintf(os.Stderr, "warning: state report is %s old (reported_at %s)\n",
		age.Truncate(time.Second),
		reportedAt.UTC().Format(time.RFC3339),
	)
}
