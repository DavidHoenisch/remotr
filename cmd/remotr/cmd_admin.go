package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v3"
)

func adminCommand() *cli.Command {
	return &cli.Command{
		Name:     "admin",
		Category: catSecurity,
		Usage:    "administrative operator workflows",
		Commands: []*cli.Command{
			{
				Name:  "credential",
				Usage: "manage operator mTLS credentials",
				Commands: []*cli.Command{
					{
						Name:      "stamp",
						Usage:     "issue a new operator credential for automation (e.g. SIEM export)",
						ArgsUsage: "[output-directory]",
						Description: withExamples("",
			"remotr admin credential stamp --label siem-collector --role security_logger --out ./siem-creds"),
						Action: actionAdminCredentialStamp,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "label", Usage: "label recorded in audit metadata (e.g. siem-collector)"},
							&cli.StringSliceFlag{Name: "role", Usage: "RBAC role to assign (repeatable; e.g. security_logger, read_only, package_manager)"},
							&cli.StringFlag{Name: "out", Usage: "directory to write cert.pem, key.pem, ca.pem, and state.json"},
							&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
						},
					},
				},
			},
		},
	}
}

func actionAdminCredentialStamp(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "admin credential stamp: %v", err)
	}
	if err := requireOperatorCLI(settings, "admin credential stamp"); err != nil {
		return err
	}

	outDir := strings.TrimSpace(c.String("out"))
	if outDir == "" {
		outDir = strings.TrimSpace(c.Args().First())
	}
	if outDir == "" {
		return exitErr(2, "admin credential stamp: output directory required (--out or argument)")
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}

	resp, err := client.CreateOperatorCredential(c.String("label"), c.StringSlice("role"))
	if err != nil {
		return apiErr(c, "admin credential stamp", err)
	}

	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}
	if err := writeCredentialFile(filepath.Join(outDir, "cert.pem"), resp.CertPEM); err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}
	if err := writeCredentialFile(filepath.Join(outDir, "key.pem"), resp.KeyPEM); err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}
	if err := writeCredentialFile(filepath.Join(outDir, "ca.pem"), resp.CAPEM); err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}
	meta, err := json.Marshal(opcreds.State{OperatorID: resp.OperatorID})
	if err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}
	if err := writeCredentialFile(filepath.Join(outDir, "state.json"), string(meta)); err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
	}

	if c.Bool("json") {
		return encodeJSON(map[string]any{
			"operator_id": resp.OperatorID,
			"label":       resp.Label,
			"roles":       resp.Roles,
			"out":         outDir,
		})
	}

	fmt.Printf("operator credential stamped: %s\n", resp.OperatorID)
	if resp.Label != "" {
		fmt.Printf("label: %s\n", resp.Label)
	}
	if len(resp.Roles) > 0 {
		fmt.Printf("roles: %s\n", strings.Join(resp.Roles, ", "))
	}
	fmt.Printf("credentials written to: %s\n", outDir)
	return nil
}

func writeCredentialFile(path, pem string) error {
	return os.WriteFile(path, []byte(pem), 0o600)
}
