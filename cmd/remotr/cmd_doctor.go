package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	"github.com/urfave/cli/v3"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:     "doctor",
		Category: catSetup,
		Usage:    "diagnose operator CLI configuration and connectivity",
		Description: withExamples("Check config, credentials, and optional server reachability.",
			"remotr doctor",
			"remotr doctor --json"),
		Action: actionDoctor,
		Flags: append(outputFlags(),
			&cli.BoolFlag{Name: "skip-network", Usage: "skip server health check"},
		),
	}
}

func actionDoctor(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return structuredErr(&cliError{
			Code: codeAPI, Title: "config resolution failed", Cause: err.Error(), exitCode: 2,
		})
	}

	report := runDoctorChecks(c, settings)

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(report); err != nil {
			return err
		}
		if !report.OK {
			return structuredErr(&cliError{
				Code: codeAPI, Title: "doctor found problems", Cause: "one or more checks failed",
				Fix: "follow the fix hints in JSON output", exitCode: 2,
			})
		}
		return nil
	}

	for _, check := range report.Checks {
		switch check.Status {
		case "ok":
			writeOK(c, "%s", check.Name)
			if check.Detail != "" {
				writeInfo("       %s\n", check.Detail)
			}
		case "warn":
			writeWarn(c, "%s", check.Name)
			if check.Detail != "" {
				writeInfo("       %s\n", check.Detail)
			}
		case "fail":
			fmt.Fprintf(os.Stderr, "%s %s\n", labelError(c, "error:"), check.Name)
			if check.Detail != "" {
				writeInfo("       %s\n", check.Detail)
			}
			if check.Fix != "" {
				writeInfo("       fix: %s\n", check.Fix)
			}
		}
	}

	if report.OK {
		writeOK(c, "doctor checks passed")
		return nil
	}
	return structuredErr(&cliError{
		Code:     codeAPI,
		Title:    "doctor found problems",
		Cause:    "one or more checks failed",
		Fix:      "follow the fix hints above",
		exitCode: 2,
	})
}

func runDoctorChecks(c *cli.Command, settings opconfig.Settings) doctorReport {
	var checks []doctorCheck
	ok := true

	// Config file
	cfgPath := settings.ConfigPath
	if cfgPath == "" {
		cfgPath = opconfig.DefaultPath()
	}
	if _, err := os.Stat(cfgPath); err != nil {
		ok = false
		checks = append(checks, doctorCheck{
			Name: "operator config", Status: "warn", Detail: "config file not found: " + cfgPath,
			Fix: "remotr config init --server-url https://remotr.example:8443",
		})
	} else {
		checks = append(checks, doctorCheck{Name: "operator config", Status: "ok", Detail: cfgPath})
	}

	// Server URL
	if settings.ServerURL == "" {
		ok = false
		checks = append(checks, doctorCheck{
			Name: "server URL", Status: "fail", Detail: "not configured",
			Fix: "remotr config init --server-url https://remotr.example:8443",
		})
	} else {
		checks = append(checks, doctorCheck{Name: "server URL", Status: "ok", Detail: settings.ServerURL})
	}

	// Credentials
	if !opcreds.Present(settings.StateDir) {
		ok = false
		checks = append(checks, doctorCheck{
			Name: "operator credentials", Status: "fail",
			Detail: fmt.Sprintf("missing in %s", settings.StateDir),
			Fix:    "remotr bootstrap --token <token> --server-url <url> --ca <ca.pem>",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name: "operator credentials", Status: "ok", Detail: settings.StateDir,
		})
	}

	// CA
	if settings.CA != "" {
		if _, err := os.Stat(settings.CA); err != nil {
			ok = false
			checks = append(checks, doctorCheck{
				Name: "CA certificate", Status: "fail", Detail: settings.CA + ": " + err.Error(),
			})
		} else {
			checks = append(checks, doctorCheck{Name: "CA certificate", Status: "ok", Detail: settings.CA})
		}
	} else if settings.ServerURL != "" && !opcreds.Present(settings.StateDir) {
		checks = append(checks, doctorCheck{
			Name: "CA certificate", Status: "warn", Detail: "not set (required for bootstrap)",
			Fix: "download from " + strings.TrimRight(settings.ServerURL, "/") + "/v1/ca.pem",
		})
	}

	// Config repo context
	if repoRoot := findConfigRepoRoot("."); repoRoot != "" {
		checks = append(checks, doctorCheck{
			Name: "configuration repository", Status: "ok",
			Detail: "detected at " + repoRoot,
		})
	}

	// Network
	if !c.Bool("skip-network") && settings.ServerURL != "" {
		check := doctorNetworkCheck(settings.ServerURL, settings.CA)
		if check.Status == "fail" {
			ok = false
		}
		checks = append(checks, check)
	}

	return doctorReport{Checks: checks, OK: ok}
}

func doctorNetworkCheck(serverURL, caPath string) doctorCheck {
	client, err := doctorHTTPClient(caPath)
	if err != nil {
		return doctorCheck{
			Name: "server reachability", Status: "fail",
			Detail: err.Error(),
			Fix:    "verify the CA certificate path in operator config",
		}
	}

	url := strings.TrimRight(serverURL, "/") + "/healthz"
	resp, err := client.Get(url)
	if err != nil {
		fix := "check network, TLS, and server URL"
		if caPath == "" {
			fix = "set ca in operator config (or download from " + strings.TrimRight(serverURL, "/") + "/v1/ca.pem)"
		}
		return doctorCheck{
			Name: "server reachability", Status: "warn",
			Detail: url + ": " + err.Error(),
			Fix:    fix,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return doctorCheck{
			Name: "server reachability", Status: "warn",
			Detail: fmt.Sprintf("%s returned HTTP %d", url, resp.StatusCode),
		}
	}
	detail := url
	if caPath != "" {
		detail = url + " (verified with " + caPath + ")"
	}
	return doctorCheck{Name: "server reachability", Status: "ok", Detail: detail}
}

func doctorHTTPClient(caPath string) (*http.Client, error) {
	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(caPath)
	if err != nil {
		return nil, fmt.Errorf("load CA for health check: %w", err)
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

func findConfigRepoRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "remotr.yaml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "fleets")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func actionRootGettingStarted(_ context.Context, _ *cli.Command) error {
	printGettingStarted()
	return nil
}
