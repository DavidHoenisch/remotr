package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func actionEndpointReserve(ctx context.Context, c *cli.Command) error {
	const cmd = "endpoint reserve"
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "%s: %v", cmd, err)
	}
	fleet, err := resolveFleet(c, cmd)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.String("id"))
	if id == "" {
		id = strings.TrimSpace(c.Args().First())
	}
	if id == "" {
		return exitErr(2, "%s: --id is required", cmd)
	}
	if err := requireOperatorCLI(settings, cmd); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "%s: %v", cmd, err)
	}

	var res admin.EndpointReservation
	err = runWithSpinner(ctx, c, "reserving endpoint", func(ctx context.Context) error {
		var reserveErr error
		res, reserveErr = client.ReserveEndpoint(ctx, fleet, id, c.Duration("ttl"))
		return reserveErr
	})
	if err != nil {
		return reservationCLIError(c, cmd, err)
	}
	if err := writeTokenOut(c.String("out"), res.Token); err != nil {
		return exitErr(1, "%s: %v", cmd, err)
	}

	quiet := effectiveQuiet(c)
	if c.Bool("json") {
		item := struct {
			EndpointID string    `json:"endpoint_id"`
			Fleet      string    `json:"fleet"`
			Token      string    `json:"token,omitempty"`
			ExpiresAt  time.Time `json:"expires_at"`
			Out        string    `json:"out,omitempty"`
		}{EndpointID: res.EndpointID, Fleet: res.Fleet, ExpiresAt: res.ExpiresAt, Out: c.String("out")}
		if !quiet {
			item.Token = res.Token
		}
		return encodeJSON(item)
	}
	fmt.Printf("reserved endpoint: %s\n", res.EndpointID)
	fmt.Printf("fleet: %s\n", res.Fleet)
	if !quiet {
		fmt.Printf("enrollment token (one-time, bound to %s): %s\n", res.EndpointID, res.Token)
	}
	fmt.Printf("expires: %s\n", res.ExpiresAt.UTC().Format(time.RFC3339))
	if c.String("out") != "" {
		fmt.Printf("token written to: %s\n", c.String("out"))
	}
	fmt.Printf("override path: endpoints/%s/manifest.yaml\n", res.EndpointID)
	return nil
}

func actionEndpointUnreserve(ctx context.Context, c *cli.Command) error {
	const cmd = "endpoint unreserve"
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "%s: %v", cmd, err)
	}
	id := strings.TrimSpace(c.Args().First())
	if id == "" {
		id = strings.TrimSpace(c.String("endpoint"))
	}
	if id == "" {
		return exitErr(2, "%s: endpoint id is required", cmd)
	}
	if err := requireOperatorCLI(settings, cmd); err != nil {
		return err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "%s: %v", cmd, err)
	}
	var res admin.EndpointUnreserveResult
	err = runWithSpinner(ctx, c, "releasing reservation", func(ctx context.Context) error {
		var unreserveErr error
		res, unreserveErr = client.UnreserveEndpoint(ctx, id, c.Bool("force"))
		return unreserveErr
	})
	if err != nil {
		return reservationCLIError(c, cmd, err)
	}
	if res.Warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", res.Warning)
	}
	if c.Bool("json") {
		return encodeJSON(res)
	}
	fmt.Printf("unreserved endpoint: %s\n", res.EndpointID)
	return nil
}

// reservationCLIError surfaces the server's explanation verbatim so the
// operator knows exactly why a reservation or release was refused.
func reservationCLIError(c *cli.Command, cmd string, err error) error {
	if errors.Is(err, admin.ErrReservationConflict) {
		msg := strings.TrimPrefix(err.Error(), admin.ErrReservationConflict.Error()+": ")
		return exitErr(1, "%s: %s", cmd, msg)
	}
	return apiErr(c, cmd, err)
}

func endpointReserveCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:      "reserve",
			Usage:     "reserve an endpoint id before the machine enrolls",
			ArgsUsage: "[endpoint-id]",
			Description: withExamples(`Reserve an endpoint ID in a fleet and print a one-time enrollment token bound to it.
Merge endpoints/<id>/manifest.yaml before the machine exists; the agent that enrolls
with this token is assigned the reserved ID and receives the override on its first Sync.
Reserving an already reserved ID re-issues its token and revokes the previous one.`,
				"remotr endpoint reserve --fleet ubuntu-servers --id gitlab-runner-01",
				"remotr endpoint reserve --fleet ubuntu-servers --id gitlab-runner-01 --out /secure/enroll.token --quiet"),
			Action: actionEndpointReserve,
			Flags: append(tokenOutputFlags(),
				&cli.StringFlag{Name: "fleet", Usage: "fleet for the reserved endpoint"},
				&cli.StringFlag{Name: "id", Usage: "endpoint id to reserve"},
				&cli.DurationFlag{Name: "ttl", Value: defaultEnrollTTL, Usage: "bound token lifetime"},
				&cli.StringFlag{Name: "out", Usage: "write token to file (mode 0600)"},
			),
		},
		{
			Name:      "unreserve",
			Usage:     "release an unclaimed endpoint reservation",
			ArgsUsage: "<endpoint-id>",
			Description: withExamples(`Release a reservation that no machine has claimed and revoke its token.
Refused when the current release already contains endpoints/<id>/, because Git sync
would be blocked until that directory is removed; --force releases it anyway.`,
				"remotr endpoint unreserve gitlab-runner-01",
				"remotr endpoint unreserve gitlab-runner-01 --force"),
			Action: actionEndpointUnreserve,
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "force", Usage: "release even if the current release contains the endpoint override"},
				&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
			},
		},
	}
}
