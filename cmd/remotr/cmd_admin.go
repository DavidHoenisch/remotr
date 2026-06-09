package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

func adminCommand() *cli.Command {
	return &cli.Command{
		Name:  "admin",
		Usage: "administrative operator workflows",
		Subcommands: []*cli.Command{
			{
				Name:  "credential",
				Usage: "manage operator mTLS credentials",
				Subcommands: []*cli.Command{
					{
						Name:      "stamp",
						Usage:     "issue a new operator credential for automation (e.g. SIEM export)",
						ArgsUsage: "[output-directory]",
						Action:    actionAdminCredentialStamp,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "label", Usage: "label recorded in audit metadata (e.g. siem-collector)"},
							&cli.StringFlag{Name: "out", Usage: "directory to write cert.pem, key.pem, and ca.pem"},
						},
					},
				},
			},
		},
	}
}

func actionAdminCredentialStamp(c *cli.Context) error {
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

	resp, err := client.CreateOperatorCredential(c.String("label"))
	if err != nil {
		return exitErr(1, "admin credential stamp: %v", err)
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

	fmt.Printf("operator credential stamped: %s\n", resp.OperatorID)
	if resp.Label != "" {
		fmt.Printf("label: %s\n", resp.Label)
	}
	fmt.Printf("credentials written to: %s\n", outDir)
	return nil
}

func writeCredentialFile(path, pem string) error {
	return os.WriteFile(path, []byte(pem), 0o600)
}
