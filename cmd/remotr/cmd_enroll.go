package main

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
)

func actionEnrollTokenCreate(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "enroll token create: %v", err)
	}
	if settings.Fleet == "" {
		return exitErr(2, "enroll token create: fleet is required (config, REMOTR_FLEET, or --fleet)")
	}
	if err := requireOperatorCLI(settings, "enroll token create"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "enroll token create: %v", err)
	}

	resp, err := client.CreateEnrollToken(settings.Fleet, c.Duration("ttl"))
	if err != nil {
		return exitErr(1, "enroll token create: %v", err)
	}
	if err := writeTokenOut(c.String("out"), resp.Token); err != nil {
		return exitErr(1, "enroll token create: %v", err)
	}

	fmt.Printf("enrollment token (one-time): %s\n", resp.Token)
	fmt.Printf("fleet: %s\n", resp.Fleet)
	fmt.Printf("expires: %s\n", resp.ExpiresAt.UTC().Format(time.RFC3339))
	if c.String("out") != "" {
		fmt.Printf("token written to: %s\n", c.String("out"))
	}
	return nil
}
