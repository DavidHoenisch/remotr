package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v2"
)

func deploymentCreateCommand() *cli.Command {
	return &cli.Command{
		Name:   "create",
		Usage:  "create a reusable deployment token",
		Action: actionDeploymentCreate,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "label", Usage: "unique label identifying this deployment token", Required: true},
			&cli.DurationFlag{Name: "ttl", Value: 365 * 24 * time.Hour, Usage: "token lifetime"},
			&cli.StringFlag{Name: "out", Usage: "write token to file (mode 0600); only chance to save the secret"},
		},
	}
}

func deploymentListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "list deployment tokens",
		Action: actionDeploymentList,
		Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
	}
}

func deploymentShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show deployment token metadata",
		ArgsUsage: "<label>",
		Action:    actionDeploymentShow,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "label", Usage: "deployment token label (alternative to positional)"},
			&cli.BoolFlag{Name: "json", Usage: "output JSON"},
		},
	}
}

func deploymentRevokeCommand() *cli.Command {
	return &cli.Command{
		Name:      "revoke",
		Usage:     "revoke a deployment token",
		ArgsUsage: "<label>",
		Action:    actionDeploymentRevoke,
		Flags:     []cli.Flag{&cli.StringFlag{Name: "label", Usage: "deployment token label (alternative to positional)"}},
	}
}

func actionDeploymentCreate(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "deployment create: %v", err)
	}
	labelValue, ok := labelFromFlagOrArg(c.String("label"), c.Args().Slice())
	if !ok {
		return exitErr(2, "deployment create: --label is required")
	}
	if settings.Fleet == "" {
		return exitErr(2, "deployment create: fleet is required (config, REMOTR_FLEET, or --fleet)")
	}
	if err := requireOperatorCLI(settings, "deployment create"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "deployment create: %v", err)
	}

	resp, err := client.CreateDeploymentToken(labelValue, settings.Fleet, c.Duration("ttl"))
	if err != nil {
		return exitErr(1, "deployment create: %v", err)
	}
	if err := writeTokenOut(c.String("out"), resp.Token); err != nil {
		return exitErr(1, "deployment create: %v", err)
	}

	fmt.Printf("deployment token (view once): %s\n", resp.Token)
	fmt.Printf("label: %s\n", resp.Label)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	if c.String("out") != "" {
		fmt.Printf("token written to: %s\n", c.String("out"))
	}
	return nil
}

func actionDeploymentList(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "deployment list: %v", err)
	}
	if err := requireOperatorCLI(settings, "deployment list"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "deployment list: %v", err)
	}

	tokens, err := client.ListDeploymentTokens()
	if err != nil {
		return exitErr(1, "deployment list: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tokens); err != nil {
			return exitErr(1, "deployment list: %v", err)
		}
		return nil
	}

	if len(tokens) == 0 {
		fmt.Println("no deployment tokens")
		return nil
	}
	for _, tok := range tokens {
		fmt.Printf("%s  fleet=%s  status=%s  expires=%s\n",
			tok.Label, tok.Fleet, deploymentTokenStatus(tok), tok.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return nil
}

func actionDeploymentShow(c *cli.Context) error {
	labelValue, ok := labelFromFlagOrArg(c.String("label"), c.Args().Slice())
	if !ok {
		return exitErr(2, "deployment show: label required")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "deployment show: %v", err)
	}
	if err := requireOperatorCLI(settings, "deployment show"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "deployment show: %v", err)
	}

	tok, err := client.GetDeploymentToken(labelValue)
	if err != nil {
		return exitErr(1, "deployment show: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tok); err != nil {
			return exitErr(1, "deployment show: %v", err)
		}
		return nil
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
	return nil
}

func actionDeploymentRevoke(c *cli.Context) error {
	labelValue, ok := labelFromFlagOrArg(c.String("label"), c.Args().Slice())
	if !ok {
		return exitErr(2, "deployment revoke: label required")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "deployment revoke: %v", err)
	}
	if err := requireOperatorCLI(settings, "deployment revoke"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "deployment revoke: %v", err)
	}
	if err := client.RevokeDeploymentToken(labelValue); err != nil {
		return exitErr(1, "deployment revoke: %v", err)
	}
	fmt.Printf("revoked deployment token %s\n", labelValue)
	return nil
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
