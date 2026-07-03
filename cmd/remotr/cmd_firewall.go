package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

func firewallCommand() *cli.Command {
	return &cli.Command{
		Name:     "firewall",
		Category: catInventory,
		Usage:    "inspect firewall audit logs and live endpoint rules",
		Commands: []*cli.Command{
			{
				Name:      "logs",
				Usage:     "show firewall audit log for an endpoint",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples("",
					"remotr firewall logs phalanx-acae925c",
					"remotr firewall logs --endpoint phalanx-acae925c --json"),
				Action: actionFirewallLogs,
				Flags: append(outputFlags(),
					endpointIDFlag(),
				),
			},
			{
				Name:      "report",
				Usage:     "show live firewall rules from an endpoint",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples("",
					"remotr firewall report phalanx-acae925c",
					"remotr firewall report --endpoint phalanx-acae925c --json"),
				Action: actionFirewallReport,
				Flags: append(outputFlags(),
					endpointIDFlag(),
				),
			},
			{
				Name:      "export",
				Usage:     "export live firewall rules to CSV",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples("",
					"remotr firewall export phalanx-acae925c --output rules.csv",
					"remotr firewall export --fleet engineering --output fleet-rules.csv"),
				Action: actionFirewallExport,
				Flags: []cli.Flag{
					endpointIDFlag(),
					fleetArgFlag(),
					&cli.StringFlag{Name: "output", Usage: "write CSV to file instead of stdout"},
					&cli.StringFlag{Name: "format", Value: "csv", Usage: "output format: csv, json"},
				},
			},
		},
	}
}

func actionFirewallLogs(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "firewall logs: %v", err)
	}
	if err := requireOperatorCLI(settings, "firewall logs"); err != nil {
		return err
	}

	endpointID, err := resolveEndpointID(c, "firewall logs")
	if err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "firewall logs: %v", err)
	}

	report, err := client.GetEndpointFirewallAudit(endpointID)
	if err != nil {
		return apiErr(c, "firewall logs", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(report)
	}

	if len(report.Report) == 0 {
		writeInfoLine("no firewall audit log available for this endpoint")
		return nil
	}

	// Pretty-print audit entries (JSON Lines expected).
	var entries []map[string]any
	if err := json.Unmarshal(report.Report, &entries); err != nil {
		// If not an array, just print the raw report.
		fmt.Println(string(report.Report))
		return nil
	}
	for _, entry := range entries {
		ts, _ := entry["timestamp"].(string)
		name, _ := entry["ruleName"].(string)
		action, _ := entry["action"].(string)
		backend, _ := entry["backend"].(string)
		wouldHave, _ := entry["wouldHave"].(string)
		enforced, _ := entry["enforced"].(bool)
		mode := "audit"
		if enforced {
			mode = "enforced"
		}
		fmt.Printf("[%s] %s (%s/%s) %s: %s\n", ts, name, action, backend, mode, wouldHave)
	}
	return nil
}

func actionFirewallReport(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "firewall report: %v", err)
	}
	if err := requireOperatorCLI(settings, "firewall report"); err != nil {
		return err
	}

	endpointID, err := resolveEndpointID(c, "firewall report")
	if err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "firewall report: %v", err)
	}

	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		return apiErr(c, "firewall report", err)
	}

	if ep.SystemInfo == nil || len(ep.SystemInfo.Report) == 0 {
		writeInfoLine("no system info available for this endpoint")
		return nil
	}

	var snapshot struct {
		Firewall struct {
			Backend   string `json:"backend,omitempty"`
			Firewalld *struct {
				DefaultZone string `json:"defaultZone,omitempty"`
				Zones       []struct {
					Name      string   `json:"name"`
					Target    string   `json:"target,omitempty"`
					Services  []string `json:"services,omitempty"`
					Ports     []string `json:"ports,omitempty"`
					Sources   []string `json:"sources,omitempty"`
					RichRules []string `json:"richRules,omitempty"`
				} `json:"zones,omitempty"`
			} `json:"firewalld,omitempty"`
			Nftables *struct {
				RawRuleset string `json:"rawRuleset,omitempty"`
			} `json:"nftables,omitempty"`
		} `json:"firewall,omitempty"`
	}
	if err := json.Unmarshal(ep.SystemInfo.Report, &snapshot); err != nil {
		return exitErr(1, "firewall report: parse system info: %v", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(snapshot.Firewall)
	}

	if snapshot.Firewall.Backend == "" {
		writeInfoLine("no firewall backend detected on this endpoint")
		return nil
	}

	fmt.Printf("Backend: %s\n", snapshot.Firewall.Backend)
	if snapshot.Firewall.Firewalld != nil {
		fmt.Printf("Default Zone: %s\n", snapshot.Firewall.Firewalld.DefaultZone)
		if len(snapshot.Firewall.Firewalld.Zones) > 0 {
			fmt.Println("\nZONE\tTARGET\tSERVICES\tPORTS\tSOURCES")
		}
		for _, z := range snapshot.Firewall.Firewalld.Zones {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
				z.Name,
				z.Target,
				strings.Join(z.Services, ","),
				strings.Join(z.Ports, ","),
				strings.Join(z.Sources, ","),
			)
		}
	}
	if snapshot.Firewall.Nftables != nil {
		fmt.Println("\nRaw ruleset:")
		fmt.Println(snapshot.Firewall.Nftables.RawRuleset)
	}
	return nil
}

func actionFirewallExport(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "firewall export: %v", err)
	}
	if err := requireOperatorCLI(settings, "firewall export"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "firewall export: %v", err)
	}

	format := strings.ToLower(strings.TrimSpace(c.String("format")))
	if format != "csv" && format != "json" {
		return exitErr(2, "firewall export: --format must be csv or json")
	}

	// Determine endpoints to export.
	var endpointIDs []string
	if fleet, ok := fleetFromFlagOrArg(c); ok {
		eps, err := client.ListEndpoints()
		if err != nil {
			return apiErr(c, "firewall export", err)
		}
		for _, ep := range eps {
			if ep.Fleet == fleet {
				endpointIDs = append(endpointIDs, ep.ID)
			}
		}
		if len(endpointIDs) == 0 {
			return exitErr(1, "firewall export: no endpoints in fleet %q", fleet)
		}
	} else {
		id, err := resolveEndpointID(c, "firewall export")
		if err != nil {
			return err
		}
		endpointIDs = []string{id}
	}

	// Collect firewall data for each endpoint.
	type row struct {
		EndpointID string
		Backend    string
		Zone       string
		Target     string
		Service    string
		Port       string
		Source     string
		RawRuleset string
	}
	var rows []row

	for _, id := range endpointIDs {
		ep, err := client.GetEndpoint(id)
		if err != nil {
			return apiErr(c, "firewall export", err)
		}
		if ep.SystemInfo == nil || len(ep.SystemInfo.Report) == 0 {
			continue
		}
		var snapshot struct {
			Firewall struct {
				Backend   string `json:"backend,omitempty"`
				Firewalld *struct {
					DefaultZone string `json:"defaultZone,omitempty"`
					Zones       []struct {
						Name      string   `json:"name"`
						Target    string   `json:"target,omitempty"`
						Services  []string `json:"services,omitempty"`
						Ports     []string `json:"ports,omitempty"`
						Sources   []string `json:"sources,omitempty"`
						RichRules []string `json:"richRules,omitempty"`
					} `json:"zones,omitempty"`
				} `json:"firewalld,omitempty"`
				Nftables *struct {
					RawRuleset string `json:"rawRuleset,omitempty"`
				} `json:"nftables,omitempty"`
			} `json:"firewall,omitempty"`
		}
		if err := json.Unmarshal(ep.SystemInfo.Report, &snapshot); err != nil {
			continue
		}

		if snapshot.Firewall.Backend == "firewalld" && snapshot.Firewall.Firewalld != nil {
			for _, z := range snapshot.Firewall.Firewalld.Zones {
				services := z.Services
				if len(services) == 0 {
					services = []string{""}
				}
				ports := z.Ports
				if len(ports) == 0 {
					ports = []string{""}
				}
				sources := z.Sources
				if len(sources) == 0 {
					sources = []string{""}
				}
				for _, svc := range services {
					for _, port := range ports {
						for _, src := range sources {
							rows = append(rows, row{
								EndpointID: id,
								Backend:    "firewalld",
								Zone:       z.Name,
								Target:     z.Target,
								Service:    svc,
								Port:       port,
								Source:     src,
							})
						}
					}
				}
			}
		} else if snapshot.Firewall.Backend == "nftables" && snapshot.Firewall.Nftables != nil {
			rows = append(rows, row{
				EndpointID: id,
				Backend:    "nftables",
				RawRuleset: snapshot.Firewall.Nftables.RawRuleset,
			})
		}
	}

	if len(rows) == 0 {
		writeInfoLine("no firewall data available for export")
		return nil
	}

	// Output destination.
	out := os.Stdout
	if path := strings.TrimSpace(c.String("output")); path != "" {
		f, err := os.Create(path)
		if err != nil {
			return exitErr(1, "firewall export: create file: %v", err)
		}
		defer f.Close()
		out = f
	}

	if format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	// CSV output.
	w := csv.NewWriter(out)
	_ = w.Write([]string{"endpoint_id", "backend", "zone", "target", "service", "port", "source", "raw_ruleset"})
	for _, r := range rows {
		_ = w.Write([]string{r.EndpointID, r.Backend, r.Zone, r.Target, r.Service, r.Port, r.Source, r.RawRuleset})
	}
	w.Flush()
	return w.Error()
}
