package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	"github.com/urfave/cli/v3"
)

func actionBootstrap(ctx context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "bootstrap: %v", err)
	}

	token, err := readFlagOrStdin(c.String("token"))
	if err != nil {
		return exitErr(2, "bootstrap: %v", err)
	}
	if err := promptBootstrapInputs(&settings, &token); err != nil {
		return exitErr(2, "bootstrap: %v", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return exitErr(2, "bootstrap: --token is required (or - for stdin)")
	}
	if settings.ServerURL == "" {
		return errServerURLMissing("bootstrap")
	}
	if settings.CA == "" {
		return errCAMissing("bootstrap")
	}

	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(settings.CA)
	if err != nil {
		return exitErr(1, "bootstrap: %v", err)
	}

	client, err := admin.NewClient(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir, tlsCfg)
	if err != nil {
		return exitErr(1, "bootstrap: %v", err)
	}

	var resp admin.BootstrapResponse
	err = runWithSpinner(ctx, c, "exchanging bootstrap token", func(ctx context.Context) error {
		var bootstrapErr error
		resp, bootstrapErr = client.Bootstrap(token)
		return bootstrapErr
	})
	if err != nil {
		return apiErr(c, "bootstrap", err)
	}

	if err := opcreds.Save(settings.StateDir, resp.OperatorID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		return exitErr(1, "bootstrap: save credentials: %v", err)
	}

	if settings.CA == "" {
		settings.CA = filepath.Join(settings.StateDir, "ca.crt")
	}
	if err := opconfig.Save(settings); err != nil {
		return exitErr(1, "bootstrap: save config: %v", err)
	}

	if c.Bool("json") {
		return encodeJSON(map[string]string{
			"operator_id": resp.OperatorID,
			"state_dir":   settings.StateDir,
			"config_path": opconfig.DefaultPath(),
		})
	}

	writeOK(c, "operator bootstrapped: %s", resp.OperatorID)
	fmt.Printf("credentials saved to: %s\n", settings.StateDir)
	fmt.Printf("config saved to: %s\n", opconfig.DefaultPath())
	return nil
}
