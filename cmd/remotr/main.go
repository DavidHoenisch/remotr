package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/scaffold"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "init":
		os.Exit(runInit(os.Args[2:]))
	case "bootstrap":
		os.Exit(runBootstrap(os.Args[2:]))
	case "enroll":
		os.Exit(runEnroll(os.Args[2:]))
	case "endpoint":
		os.Exit(runEndpoint(os.Args[2:]))
	case "version", "-v", "--version":
		os.Exit(runVersion())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fleet := fs.String("fleet", "default", "initial fleet name (fleets/<fleet>/desired.yaml)")
	policy := fs.String("policy", "auto", "fleet remediation policy: auto or report")
	register := fs.Bool("register-server", false, "register fleet in Postgres (REMOTR_DATABASE_URL or --database-url)")
	dbURL := fs.String("database-url", "", "Postgres URL for --register-server (default: REMOTR_DATABASE_URL)")
	enroll := fs.Bool("enroll", false, "with --register-server, create a one-time enrollment token")
	enrollTTL := fs.Duration("enroll-ttl", 7*24*time.Hour, "enrollment token lifetime")
	enrollOut := fs.String("enroll-out", "", "write enrollment token to this file (mode 0600)")

	dir, flagArgs := splitInitArgs(args)
	_ = fs.Parse(flagArgs)
	if dir == "" {
		dir = "."
	}

	url := strings.TrimSpace(*dbURL)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("REMOTR_DATABASE_URL"))
	}
	if *enroll && !*register {
		fmt.Fprintln(os.Stderr, "init: --enroll requires --register-server")
		return 2
	}

	res, err := scaffold.Init(context.Background(), scaffold.Options{
		Dir:               dir,
		Fleet:             *fleet,
		RemediationPolicy: *policy,
		RegisterServer:    *register,
		DatabaseURL:       url,
		CreateEnrollToken: *enroll,
		EnrollTokenTTL:    *enrollTTL,
		EnrollTokenOut:    *enrollOut,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}

	fmt.Printf("created configuration repository at %s\n", res.Dir)
	fmt.Printf("  fleet: fleets/%s/desired.yaml\n", res.Fleet)
	if res.EnrollToken != "" {
		fmt.Printf("  enrollment token (one-time): %s\n", res.EnrollToken)
		fmt.Printf("  expires: %s\n", res.EnrollExpires.UTC().Format(time.RFC3339))
		if *enrollOut != "" {
			fmt.Printf("  token written to: %s\n", *enrollOut)
		}
	}
	fmt.Println()
	fmt.Println("Next: git init, push, set REMOTR_CONFIG_REPO on the server, enroll agents.")
	return 0
}

func runBootstrap(args []string) int {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	serverURL := fs.String("server-url", "", "Remotr server base URL (https://host:8443)")
	ca := fs.String("ca", "", "Remotr CA certificate PEM file")
	token := fs.String("token", "", "one-time bootstrap token from server startup")
	stateDir := fs.String("state-dir", opcreds.DefaultDir(), "directory for operator credentials")
	_ = fs.Parse(args)

	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*ca) == "" || strings.TrimSpace(*token) == "" {
		fmt.Fprintln(os.Stderr, "bootstrap: --server-url, --ca, and --token are required")
		return 2
	}

	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(*ca)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 1
	}

	client := admin.NewClient(strings.TrimRight(*serverURL, "/"), *stateDir, tlsCfg)
	resp, err := client.Bootstrap(strings.TrimSpace(*token))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 1
	}

	if err := opcreds.Save(*stateDir, resp.OperatorID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: save credentials: %v\n", err)
		return 1
	}

	fmt.Printf("operator bootstrapped: %s\n", resp.OperatorID)
	fmt.Printf("credentials saved to: %s\n", *stateDir)
	return 0
}

func runEnroll(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "enroll: subcommand required (token create)")
		return 2
	}
	switch args[0] {
	case "token":
		return runEnrollToken(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown enroll subcommand %q\n", args[0])
		return 2
	}
}

func runEnrollToken(args []string) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: remotr enroll token create --fleet <name> [--ttl duration]")
		return 2
	}
	fs := flag.NewFlagSet("enroll token create", flag.ExitOnError)
	serverURL := fs.String("server-url", "", "Remotr server base URL")
	fleet := fs.String("fleet", "", "fleet name for enrollment token")
	ttl := fs.Duration("ttl", 7*24*time.Hour, "token lifetime")
	stateDir := fs.String("state-dir", opcreds.DefaultDir(), "operator credentials directory")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*fleet) == "" {
		fmt.Fprintln(os.Stderr, "enroll token create: --server-url and --fleet are required")
		return 2
	}
	if !opcreds.Present(*stateDir) {
		fmt.Fprintf(os.Stderr, "enroll token create: operator credentials missing in %s (run remotr bootstrap first)\n", *stateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(*serverURL, "/"), *stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 1
	}

	resp, err := client.CreateEnrollToken(*fleet, *ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 1
	}

	fmt.Printf("enrollment token (one-time): %s\n", resp.Token)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	return 0
}

func runEndpoint(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "endpoint: subcommand required (list, show)")
		return 2
	}
	switch args[0] {
	case "list":
		return runEndpointList(args[1:])
	case "show":
		return runEndpointShow(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown endpoint subcommand %q\n", args[0])
		return 2
	}
}

func runEndpointList(args []string) int {
	fs := flag.NewFlagSet("endpoint list", flag.ExitOnError)
	serverURL := fs.String("server-url", "", "Remotr server base URL")
	stateDir := fs.String("state-dir", opcreds.DefaultDir(), "operator credentials directory")
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	if strings.TrimSpace(*serverURL) == "" {
		fmt.Fprintln(os.Stderr, "endpoint list: --server-url is required")
		return 2
	}
	if !opcreds.Present(*stateDir) {
		fmt.Fprintf(os.Stderr, "endpoint list: operator credentials missing in %s (run remotr bootstrap first)\n", *stateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(*serverURL, "/"), *stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint list: %v\n", err)
		return 1
	}

	eps, err := client.ListEndpoints()
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint list: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(eps); err != nil {
			fmt.Fprintf(os.Stderr, "endpoint list: %v\n", err)
			return 1
		}
		return 0
	}

	if len(eps) == 0 {
		fmt.Println("no endpoints enrolled")
		return 0
	}
	for _, ep := range eps {
		fmt.Printf("%s\tfleet=%s", ep.ID, ep.Fleet)
		if ep.CertFingerprint != "" {
			fmt.Printf("\tfp=%s", ep.CertFingerprint)
		}
		if len(ep.Labels) > 0 {
			fmt.Printf("\tlabels=%s", formatLabels(ep.Labels))
		}
		fmt.Println()
	}
	return 0
}

func runEndpointShow(args []string) int {
	fs := flag.NewFlagSet("endpoint show", flag.ExitOnError)
	serverURL := fs.String("server-url", "", "Remotr server base URL")
	stateDir := fs.String("state-dir", opcreds.DefaultDir(), "operator credentials directory")
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	if len(fs.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "usage: remotr endpoint show --server-url URL <endpoint-id>")
		return 2
	}
	endpointID := fs.Args()[0]

	if strings.TrimSpace(*serverURL) == "" {
		fmt.Fprintln(os.Stderr, "endpoint show: --server-url is required")
		return 2
	}
	if !opcreds.Present(*stateDir) {
		fmt.Fprintf(os.Stderr, "endpoint show: operator credentials missing in %s (run remotr bootstrap first)\n", *stateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(*serverURL, "/"), *stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint show: %v\n", err)
		return 1
	}

	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint show: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(ep); err != nil {
			fmt.Fprintf(os.Stderr, "endpoint show: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Printf("id: %s\n", ep.ID)
	fmt.Printf("fleet: %s\n", ep.Fleet)
	if ep.CertFingerprint != "" {
		fmt.Printf("cert_fingerprint: %s\n", ep.CertFingerprint)
	}
	if len(ep.Labels) > 0 {
		fmt.Printf("labels: %s\n", formatLabels(ep.Labels))
	} else {
		fmt.Println("labels: (none)")
	}
	if ep.LastDrift != nil {
		fmt.Printf("last_drift:\n")
		fmt.Printf("  release_ref: %s\n", ep.LastDrift.ReleaseRef)
		fmt.Printf("  digest: %s\n", ep.LastDrift.Digest)
		fmt.Printf("  reported_at: %s\n", ep.LastDrift.ReportedAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("last_drift: (none)")
	}
	return 0
}

func formatLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}

// splitInitArgs separates directory (first non-flag token) from flags (any order).
func splitInitArgs(args []string) (dir string, flags []string) {
	needsValue := map[string]bool{
		"-fleet": true, "-policy": true, "-database-url": true,
		"-enroll-ttl": true, "-enroll-out": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			key := a
			if strings.Contains(a, "=") {
				continue
			}
			if needsValue[key] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if dir == "" {
			dir = a
			continue
		}
		flags = append(flags, a)
	}
	return dir, flags
}

func runVersion() int {
	if commit != "" {
		fmt.Printf("remotr %s (%s", version, commit)
		if date != "" {
			fmt.Printf(", %s", date)
		}
		fmt.Println(")")
		return 0
	}
	fmt.Printf("remotr %s\n", version)
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `remotr — operator CLI for Remotr (GitOps config + server registry)

Usage:
  remotr init [directory] [flags]
  remotr bootstrap --server-url URL --ca PATH --token TOKEN [flags]
  remotr enroll token create --server-url URL --fleet NAME [--ttl duration]
  remotr endpoint list --server-url URL [--json]
  remotr endpoint show --server-url URL <endpoint-id> [--json]
  remotr version

Scaffold a new configuration repository (Git source of truth) with sample
fleets/<fleet>/desired.yaml, operator metadata, and server.env.example.

Flags for init:
  -fleet string          initial fleet name (default "default")
  -policy string         remediation policy: auto or report (default "auto")
  -register-server       upsert fleet in Postgres (needs database URL)
  -database-url string   Postgres URL (default REMOTR_DATABASE_URL)
  -enroll                create a one-time enrollment token (with -register-server)
  -enroll-ttl duration   token lifetime (default 168h)
  -enroll-out path       write token to file

Flags for bootstrap:
  -server-url string     Remotr server base URL
  -ca path               Remotr CA certificate PEM
  -token string          one-time bootstrap token
  -state-dir path        operator credential directory (default ~/.config/remotr)

Examples:
  remotr init -fleet engineering ./remotr-config
  remotr bootstrap --server-url https://127.0.0.1:8443 --ca ./ca.crt --token "$TOKEN"
  remotr enroll token create --server-url https://127.0.0.1:8443 --fleet engineering
  remotr endpoint list --server-url https://127.0.0.1:8443
  remotr endpoint show --server-url https://127.0.0.1:8443 <endpoint-id>

`)
}
