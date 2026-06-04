package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
	"github.com/urfave/cli/v2"
)

func actionBootstrap(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "bootstrap: %v", err)
	}
	token := strings.TrimSpace(c.String("token"))
	if token == "" {
		return exitErr(2, "bootstrap: --token is required")
	}
	if settings.ServerURL == "" {
		return exitErr(2, "bootstrap: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if settings.CA == "" {
		return exitErr(2, "bootstrap: CA path is required (config, REMOTR_CA, --ca, or ca.crt in state-dir)")
	}

	tlsCfg, err := tlsconfig.TrustOnlyTLSConfig(settings.CA)
	if err != nil {
		return exitErr(1, "bootstrap: %v", err)
	}

	client, err := admin.NewClient(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir, tlsCfg)
	if err != nil {
		return exitErr(1, "bootstrap: %v", err)
	}
	resp, err := client.Bootstrap(token)
	if err != nil {
		return exitErr(1, "bootstrap: %v", err)
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

	fmt.Printf("operator bootstrapped: %s\n", resp.OperatorID)
	fmt.Printf("credentials saved to: %s\n", settings.StateDir)
	fmt.Printf("config saved to: %s\n", opconfig.DefaultPath())
	return nil
}
