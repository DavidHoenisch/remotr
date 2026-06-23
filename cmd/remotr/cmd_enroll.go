package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

func actionEnrollTokenCreate(ctx context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "enroll token create: %v", err)
	}
	fleet := settings.Fleet
	if err := promptFleet(&fleet); err != nil {
		return exitErr(2, "enroll token create: %v", err)
	}
	settings.Fleet = fleet
	if settings.Fleet == "" {
		return errFleetMissing("enroll token create")
	}
	if err := requireOperatorCLI(settings, "enroll token create"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "enroll token create: %v", err)
	}

	var token, fleetName string
	var expiresAt time.Time
	err = runWithSpinner(ctx, c, "creating enrollment token", func(ctx context.Context) error {
		resp, createErr := client.CreateEnrollToken(settings.Fleet, c.Duration("ttl"))
		if createErr != nil {
			return createErr
		}
		token = resp.Token
		fleetName = resp.Fleet
		expiresAt = resp.ExpiresAt
		return nil
	})
	if err != nil {
		return apiErr(c, "enroll token create", err)
	}
	if err := writeTokenOut(c.String("out"), token); err != nil {
		return exitErr(1, "enroll token create: %v", err)
	}

	quiet := effectiveQuiet(c)
	if c.Bool("json") {
		type out struct {
			Token     string    `json:"token,omitempty"`
			Fleet     string    `json:"fleet"`
			ExpiresAt time.Time `json:"expires_at"`
			Out       string    `json:"out,omitempty"`
		}
		item := out{Fleet: fleetName, ExpiresAt: expiresAt}
		if !quiet {
			item.Token = token
		}
		if path := c.String("out"); path != "" {
			item.Out = path
		}
		return encodeJSON(item)
	}

	if !quiet {
		fmt.Printf("enrollment token (one-time): %s\n", token)
	}
	fmt.Printf("fleet: %s\n", fleetName)
	fmt.Printf("expires: %s\n", expiresAt.UTC().Format(time.RFC3339))
	if c.String("out") != "" {
		fmt.Printf("token written to: %s\n", c.String("out"))
	}
	return nil
}
