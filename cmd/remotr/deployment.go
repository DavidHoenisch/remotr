package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func runDeployment(args []string) int {
	if len(args) == 0 {
		printDeploymentUsage()
		return 2
	}
	switch args[0] {
	case "create":
		return runDeploymentCreate(args[1:])
	case "list":
		return runDeploymentList(args[1:])
	case "show":
		return runDeploymentShow(args[1:])
	case "revoke":
		return runDeploymentRevoke(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown deployment subcommand %q\n", args[0])
		printDeploymentUsage()
		return 2
	}
}

func printDeploymentUsage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment create --label NAME [--fleet NAME] [--ttl duration] [--out path]")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment list [--json]")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment show [--label NAME] <label>")
	fmt.Fprintln(os.Stderr, "  remotr enroll deployment revoke [--label NAME] <label>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Alias: remotr deployment <subcommand> ...")
}

func runDeploymentCreate(args []string) int {
	fs := flag.NewFlagSet("deployment create", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	label := fs.String("label", "", "unique label identifying this deployment token")
	ttl := fs.Duration("ttl", 365*24*time.Hour, "token lifetime")
	out := fs.String("out", "", "write token to file (mode 0600); only chance to save the secret")
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment create: %v\n", err)
		return 2
	}
	labelValue, ok := labelFromFlagOrArg(*label, fs.Args())
	if !ok {
		fmt.Fprintln(os.Stderr, "deployment create: --label is required")
		return 2
	}
	if settings.Fleet == "" {
		fmt.Fprintln(os.Stderr, "deployment create: fleet is required (config, REMOTR_FLEET, or --fleet)")
		return 2
	}
	if !requireOperatorCLI(settings, "deployment create") {
		return 2
	}

	client, err := newAdminClient(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment create: %v\n", err)
		return 1
	}

	resp, err := client.CreateDeploymentToken(labelValue, settings.Fleet, *ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment create: %v\n", err)
		return 1
	}
	if err := writeTokenOut(*out, resp.Token); err != nil {
		fmt.Fprintf(os.Stderr, "deployment create: %v\n", err)
		return 1
	}

	fmt.Printf("deployment token (view once): %s\n", resp.Token)
	fmt.Printf("label: %s\n", resp.Label)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	if *out != "" {
		fmt.Printf("token written to: %s\n", *out)
	}
	return 0
}

func runDeploymentList(args []string) int {
	fs := flag.NewFlagSet("deployment list", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment list: %v\n", err)
		return 2
	}
	if !requireOperatorCLI(settings, "deployment list") {
		return 2
	}

	client, err := newAdminClient(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment list: %v\n", err)
		return 1
	}

	tokens, err := client.ListDeploymentTokens()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment list: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tokens); err != nil {
			fmt.Fprintf(os.Stderr, "deployment list: %v\n", err)
			return 1
		}
		return 0
	}

	if len(tokens) == 0 {
		fmt.Println("no deployment tokens")
		return 0
	}

	for _, tok := range tokens {
		fmt.Printf("%s  fleet=%s  status=%s  expires=%s\n",
			tok.Label, tok.Fleet, deploymentTokenStatus(tok), tok.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return 0
}

func runDeploymentShow(args []string) int {
	fs := flag.NewFlagSet("deployment show", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	label := fs.String("label", "", "deployment token label")
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)

	labelValue, ok := labelFromFlagOrArg(*label, fs.Args())
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: remotr enroll deployment show [flags] <label>")
		return 2
	}

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment show: %v\n", err)
		return 2
	}
	if !requireOperatorCLI(settings, "deployment show") {
		return 2
	}

	client, err := newAdminClient(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment show: %v\n", err)
		return 1
	}

	tok, err := client.GetDeploymentToken(labelValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment show: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tok); err != nil {
			fmt.Fprintf(os.Stderr, "deployment show: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Printf("label: %s\n", tok.Label)
	fmt.Printf("id: %s\n", tok.ID)
	fmt.Printf("fleet: %s\n", tok.Fleet)
	fmt.Printf("status: %s\n", deploymentTokenStatus(tok))
	fmt.Printf("created: %s\n", tok.CreatedAt.UTC().Format(time.RFC3339))
	fmt.Printf("expires: %s\n", tok.ExpiresAt.UTC().Format(time.RFC3339))
	if tok.RevokedAt != nil {
		fmt.Printf("revoked: %s\n", tok.RevokedAt.UTC().Format(time.RFC3339))
	}
	if tok.LastUsedAt != nil {
		fmt.Printf("last used: %s\n", tok.LastUsedAt.UTC().Format(time.RFC3339))
	}
	return 0
}

func runDeploymentRevoke(args []string) int {
	fs := flag.NewFlagSet("deployment revoke", flag.ExitOnError)
	var cfg commonConfigFlags
	bindCommonConfigFlags(fs, &cfg)
	label := fs.String("label", "", "deployment token label")
	_ = fs.Parse(args)

	labelValue, ok := labelFromFlagOrArg(*label, fs.Args())
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: remotr enroll deployment revoke [flags] <label>")
		return 2
	}

	settings, err := cfg.resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment revoke: %v\n", err)
		return 2
	}
	if !requireOperatorCLI(settings, "deployment revoke") {
		return 2
	}

	client, err := newAdminClient(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment revoke: %v\n", err)
		return 1
	}

	if err := client.RevokeDeploymentToken(labelValue); err != nil {
		fmt.Fprintf(os.Stderr, "deployment revoke: %v\n", err)
		return 1
	}

	fmt.Printf("revoked deployment token %s\n", labelValue)
	return 0
}

func deploymentTokenStatus(tok admin.DeploymentToken) string {
	if tok.RevokedAt != nil {
		return "revoked"
	}
	if time.Now().After(tok.ExpiresAt) {
		return "expired"
	}
	return "active"
}
