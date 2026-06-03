package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/scaffold"
	"github.com/urfave/cli/v2"
)

const defaultEnrollTTL = 7 * 24 * time.Hour

func actionInit(c *cli.Context) error {
	dir := c.Args().First()
	if dir == "" {
		dir = "."
	}

	url := strings.TrimSpace(c.String("database-url"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("REMOTR_DATABASE_URL"))
	}
	if c.Bool("enroll") && !c.Bool("register-server") {
		return exitErr(2, "init: --enroll requires --register-server")
	}

	res, err := scaffold.Init(context.Background(), scaffold.Options{
		Dir:               dir,
		Fleet:             c.String("fleet"),
		RemediationPolicy: c.String("policy"),
		RegisterServer:    c.Bool("register-server"),
		DatabaseURL:       url,
		CreateEnrollToken: c.Bool("enroll"),
		EnrollTokenTTL:    c.Duration("enroll-ttl"),
		EnrollTokenOut:    c.String("enroll-out"),
	})
	if err != nil {
		return exitErr(1, "init: %v", err)
	}

	fmt.Printf("created configuration repository at %s\n", res.Dir)
	fmt.Printf("  fleet: fleets/%s/desired.yaml\n", res.Fleet)
	if res.EnrollToken != "" {
		fmt.Printf("  enrollment token (one-time): %s\n", res.EnrollToken)
		fmt.Printf("  expires: %s\n", res.EnrollExpires.UTC().Format(time.RFC3339))
		if c.String("enroll-out") != "" {
			fmt.Printf("  token written to: %s\n", c.String("enroll-out"))
		}
	}
	fmt.Println()
	fmt.Println("Next: git init, push, set REMOTR_CONFIG_REPO on the server, enroll agents.")
	return nil
}
