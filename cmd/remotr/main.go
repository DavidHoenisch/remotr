package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
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
	case "config":
		os.Exit(runConfig(os.Args[2:]))
	case "git":
		os.Exit(runGit(os.Args[2:]))
	case "deployment":
		os.Exit(runDeployment(os.Args[2:]))
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
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	token := fs.String("token", "", "one-time bootstrap token from server startup")
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 2
	}
	if strings.TrimSpace(*token) == "" {
		fmt.Fprintln(os.Stderr, "bootstrap: --token is required")
		return 2
	}
	if settings.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "bootstrap: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
		return 2
	}
	if settings.CA == "" {
		fmt.Fprintln(os.Stderr, "bootstrap: CA path is required (config, REMOTR_CA, --ca, or ca.crt in state-dir)")
		return 2
	}

	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(settings.CA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 1
	}

	client := admin.NewClient(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir, tlsCfg)
	resp, err := client.Bootstrap(strings.TrimSpace(*token))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 1
	}

	if err := opcreds.Save(settings.StateDir, resp.OperatorID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: save credentials: %v\n", err)
		return 1
	}

	if settings.CA == "" {
		settings.CA = filepath.Join(settings.StateDir, "ca.crt")
	}
	if err := opconfig.Save(settings); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: save config: %v\n", err)
		return 1
	}

	fmt.Printf("operator bootstrapped: %s\n", resp.OperatorID)
	fmt.Printf("credentials saved to: %s\n", settings.StateDir)
	fmt.Printf("config saved to: %s\n", opconfig.DefaultPath())
	return 0
}

func runEnroll(args []string) int {
	if len(args) == 0 {
		printEnrollUsage()
		return 2
	}
	switch args[0] {
	case "token":
		return runEnrollToken(args[1:])
	case "deployment":
		return runDeployment(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown enroll subcommand %q\n", args[0])
		printEnrollUsage()
		return 2
	}
}

func printEnrollUsage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  remotr enroll token create [--fleet NAME] [--ttl duration] [--out path]")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment create --label NAME [--fleet NAME] [--ttl duration] [--out path]")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment list [--json]")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment show [--label NAME] <label>")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment revoke [--label NAME] <label>")
}

func runEnrollToken(args []string) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: remotr enroll token create --fleet <name> [--ttl duration]")
		return 2
	}
	fs := flag.NewFlagSet("enroll token create", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	ttl := fs.Duration("ttl", 7*24*time.Hour, "token lifetime")
	out := fs.String("out", "", "write token to file (mode 0600)")
	_ = fs.Parse(args[1:])

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 2
	}
	if settings.Fleet == "" {
		fmt.Fprintln(os.Stderr, "enroll token create: fleet is required (config, REMOTR_FLEET, or --fleet)")
		return 2
	}
	if !requireOperatorCLI(settings, "enroll token create") {
		return 2
	}

	client, err := newAdminClient(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 1
	}

	resp, err := client.CreateEnrollToken(settings.Fleet, *ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 1
	}
	if err := writeTokenOut(*out, resp.Token); err != nil {
		fmt.Fprintf(os.Stderr, "enroll token create: %v\n", err)
		return 1
	}

	fmt.Printf("enrollment token (one-time): %s\n", resp.Token)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	if *out != "" {
		fmt.Printf("token written to: %s\n", *out)
	}
	return 0
}

func runGit(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: remotr git sync")
		return 2
	}
	switch args[0] {
	case "sync":
		return runGitSync(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown git subcommand %q\n", args[0])
		return 2
	}
}

func runGitSync(args []string) int {
	fs := flag.NewFlagSet("git sync", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git sync: %v\n", err)
		return 2
	}
	if settings.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "git sync: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
		return 2
	}
	if !opcreds.Present(settings.StateDir) {
		fmt.Fprintf(os.Stderr, "git sync: operator credentials missing in %s (run remotr bootstrap first)\n", settings.StateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git sync: %v\n", err)
		return 1
	}
	if err := client.TriggerGitSync(); err != nil {
		fmt.Fprintf(os.Stderr, "git sync: %v\n", err)
		return 1
	}
	fmt.Println("git sync ok")
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
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint list: %v\n", err)
		return 2
	}
	if settings.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "endpoint list: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
		return 2
	}
	if !opcreds.Present(settings.StateDir) {
		fmt.Fprintf(os.Stderr, "endpoint list: operator credentials missing in %s (run remotr bootstrap first)\n", settings.StateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
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
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	if len(fs.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "usage: remotr endpoint show [flags] <endpoint-id>")
		return 2
	}
	endpointID := fs.Args()[0]

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint show: %v\n", err)
		return 2
	}
	if settings.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "endpoint show: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
		return 2
	}
	if !opcreds.Present(settings.StateDir) {
		fmt.Fprintf(os.Stderr, "endpoint show: operator credentials missing in %s (run remotr bootstrap first)\n", settings.StateDir)
		return 2
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
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

type commonConfigFlags struct {
	configPath *string
	serverURL  *string
	stateDir   *string
	ca         *string
	fleet      *string
}

func bindCommonConfigFlags(fs *flag.FlagSet, cfg *commonConfigFlags) {
	cfg.configPath = fs.String("config", "", "operator config file (default ~/.config/remotr/config.yaml)")
	cfg.serverURL = fs.String("server-url", "", "Remotr server base URL")
	cfg.stateDir = fs.String("state-dir", "", "operator credentials directory")
	cfg.ca = fs.String("ca", "", "Remotr CA certificate PEM file")
	cfg.fleet = fs.String("fleet", "", "default fleet name")
}

func (cfg commonConfigFlags) resolve() (opconfig.Settings, error) {
	return opconfig.Resolve(*cfg.configPath, *cfg.serverURL, *cfg.stateDir, *cfg.ca, *cfg.fleet)
}

func runConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: remotr config (show|path|init)")
		return 2
	}
	switch args[0] {
	case "show":
		return runConfigShow(args[1:])
	case "path":
		fmt.Println(opconfig.DefaultPath())
		return 0
	case "init":
		return runConfigInit(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", args[0])
		return 2
	}
}

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config show: %v\n", err)
		return 2
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		fmt.Fprintf(os.Stderr, "config show: %v\n", err)
		return 1
	}
	return 0
}

func runConfigInit(args []string) int {
	fs := flag.NewFlagSet("config init", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config init: %v\n", err)
		return 2
	}
	if settings.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "config init: set server URL via --server-url or REMOTR_SERVER_URL")
		return 2
	}

	if err := opconfig.Save(settings); err != nil {
		fmt.Fprintf(os.Stderr, "config init: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s\n", opconfig.DefaultPath())
	return 0
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
  remotr bootstrap [--token TOKEN] [flags]
  remotr enroll token create [--fleet NAME] [--ttl duration] [--out path]
  remotr enroll deployment create --label NAME [--fleet NAME] [--ttl duration] [--out path]
  remotr enroll deployment list [--json]
  remotr enroll deployment show [--label NAME] <label>
  remotr enroll deployment revoke [--label NAME] <label>
  remotr deployment <subcommand> ...   (alias for enroll deployment)
  remotr endpoint list [flags]
  remotr endpoint show [flags] <endpoint-id>
  remotr git sync [flags]
  remotr config (show|path|init)
  remotr version

Configuration:
  Defaults load from ~/.config/remotr/config.yaml (override with --config or REMOTR_CONFIG).
  Precedence: flags > environment > config file > built-in defaults.

  server_url   Remotr server base URL (REMOTR_SERVER_URL)
  state_dir    Operator credentials directory (REMOTR_OPERATOR_STATE_DIR)
  ca           Remotr CA certificate PEM (REMOTR_CA; defaults to state_dir/ca.crt)
  fleet        Default fleet for enroll token create (REMOTR_FLEET)

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

Shared flags (bootstrap, enroll, endpoint, git, config):
  -config path           config file (default ~/.config/remotr/config.yaml)
  -server-url string     Remotr server base URL
  -state-dir path        operator credential directory
  -ca path               Remotr CA certificate PEM (bootstrap)
  -fleet string          fleet name (enroll token/deployment create)

Enroll flags:
  -label string          deployment token label (create; required)
  -ttl duration          token lifetime (one-time default 168h; deployment default 8760h)
  -out path              write token secret to file (mode 0600)

Examples:
  remotr config init --server-url https://remotr.example.fly.dev --state-dir ~/.config/remotr/prod
  remotr bootstrap --token "$TOKEN"
  remotr enroll token create --fleet engineering --out /secure/enroll.token
  remotr enroll deployment create --label prod-agents --fleet engineering --out /secure/deploy.token
  remotr enroll deployment list
  remotr enroll deployment show prod-agents
  remotr enroll deployment revoke prod-agents
  remotr git sync
  remotr endpoint list
  remotr endpoint show <endpoint-id>

`)
}
