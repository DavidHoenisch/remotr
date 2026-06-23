package main

import (
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

func deploymentSubcommands() []*cli.Command {
	return []*cli.Command{
		deploymentCreateCommand(),
		deploymentListCommand(),
		deploymentShowCommand(),
		deploymentRevokeCommand(),
	}
}

func deploymentCreateCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create a reusable deployment token",
		Description: withExamples("",
			"remotr deployment create --label prod-laptops --fleet production --out /secure/deploy.token --quiet"),
		Action: actionDeploymentCreate,
		Flags: append(tokenOutputFlags(),
			&cli.StringFlag{Name: "label", Usage: "unique label identifying this deployment token"},
			&cli.DurationFlag{Name: "ttl", Value: 365 * 24 * time.Hour, Usage: "token lifetime"},
			&cli.StringFlag{Name: "out", Usage: "write token to file (mode 0600); only chance to save the secret"},
		),
	}
}

func deploymentListCommand() *cli.Command {
	return &cli.Command{
		Name:     "list",
		Usage:    "list deployment tokens",
		Description: withExamples("",
			"remotr deployment list", "remotr deployment list --json"),
		Action:   actionDeploymentList,
		Flags:    outputFlags(),
	}
}

func deploymentShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show deployment token metadata",
		ArgsUsage: "[label]",
		Description: withExamples("",
			"remotr deployment show prod-laptops",
			"remotr deployment show --label prod-laptops --json"),
		Action: actionDeploymentShow,
		Flags: append(outputFlags(),
			&cli.StringFlag{Name: "label", Usage: "deployment token label (alternative to positional)"},
		),
	}
}

func deploymentRevokeCommand() *cli.Command {
	return &cli.Command{
		Name:      "revoke",
		Usage:     "revoke a deployment token",
		ArgsUsage: "[label]",
		Description: withExamples(`Revoke a deployment token. Requires --confirm matching the label.`,
			"remotr deployment revoke prod-laptops --confirm prod-laptops"),
		Action: actionDeploymentRevoke,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "label", Usage: "deployment token label (alternative to positional)"},
			confirmFlag("label"),
		},
	}
}

func actionDeploymentCreate(ctx context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "deployment create: %v", err)
	}
	labelValue, err := resolveLabel(c, "deployment create", "label", "Deployment token label")
	if err != nil {
		return err
	}
	if settings.Fleet == "" {
		return errFleetMissing("deployment create")
	}
	if err := requireOperatorCLI(settings, "deployment create"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "deployment create: %v", err)
	}

	var resp admin.CreateDeploymentTokenResponse
	err = runWithSpinner(ctx, c, "creating deployment token", func(ctx context.Context) error {
		r, createErr := client.CreateDeploymentToken(labelValue, settings.Fleet, c.Duration("ttl"))
		if createErr != nil {
			return createErr
		}
		resp = r
		return nil
	})
	if err != nil {
		return apiErr(c, "deployment create", err)
	}
	if err := writeTokenOut(c.String("out"), resp.Token); err != nil {
		return exitErr(1, "deployment create: %v", err)
	}

	if c.Bool("json") {
		type out struct {
			Token     string    `json:"token,omitempty"`
			Label     string    `json:"label"`
			Fleet     string    `json:"fleet"`
			ExpiresAt time.Time `json:"expires_at"`
			Out       string    `json:"out,omitempty"`
		}
		item := out{Label: resp.Label, Fleet: resp.Fleet, ExpiresAt: resp.ExpiresAt}
		if !effectiveQuiet(c) {
			item.Token = resp.Token
		}
		if path := c.String("out"); path != "" {
			item.Out = path
		}
		return encodeJSON(item)
	}

	if !effectiveQuiet(c) {
		fmt.Printf("deployment token (view once): %s\n", resp.Token)
	}
	fmt.Printf("label: %s\n", resp.Label)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	if c.String("out") != "" {
		fmt.Printf("token written to: %s\n", c.String("out"))
	}
	return nil
}

func actionDeploymentList(_ context.Context, c *cli.Command) error {
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
		return apiErr(c, "deployment list", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(tokens)
	}

	if len(tokens) == 0 {
		writeInfoLine("no deployment tokens")
		return nil
	}

	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("LABEL\tFLEET\tSTATUS\tEXPIRES")
	}
	for _, tok := range tokens {
		fmt.Printf("%s\t%s\t%s\t%s\n",
			tok.Label, tok.Fleet, deploymentTokenStatus(tok), tok.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return nil
}

func actionDeploymentShow(_ context.Context, c *cli.Command) error {
	labelValue, err := resolveLabel(c, "deployment show", "label", "Deployment token label")
	if err != nil {
		return err
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
		return apiErr(c, "deployment show", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(tok)
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

func actionDeploymentRevoke(_ context.Context, c *cli.Command) error {
	labelValue, err := resolveLabel(c, "deployment revoke", "label", "Deployment token label")
	if err != nil {
		return err
	}
	if err := requireConfirm(c, "deployment revoke", labelValue); err != nil {
		return err
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
		return apiErr(c, "deployment revoke", err)
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
