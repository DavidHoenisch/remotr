package main

import (
	"fmt"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/urfave/cli/v2"
)

func commonConfigFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "config",
			Usage: "operator config file (default ~/.config/remotr/config.yaml)",
		},
		&cli.StringFlag{
			Name:  "server-url",
			Usage: "Remotr server base URL",
		},
		&cli.StringFlag{
			Name:  "state-dir",
			Usage: "operator credentials directory",
		},
		&cli.StringFlag{
			Name:  "ca",
			Usage: "Remotr CA certificate PEM file",
		},
		&cli.StringFlag{
			Name:  "fleet",
			Usage: "default fleet name",
		},
	}
}

func resolveSettings(c *cli.Context) (opconfig.Settings, error) {
	return opconfig.Resolve(
		c.String("config"),
		c.String("server-url"),
		c.String("state-dir"),
		c.String("ca"),
		c.String("fleet"),
	)
}

func exitErr(code int, format string, args ...any) error {
	return cli.Exit(fmt.Sprintf(format, args...), code)
}
