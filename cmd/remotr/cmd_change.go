package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

func changeCommand() *cli.Command {
	return &cli.Command{
		Name: "change", Category: catSecurity, Usage: "review and control high-risk desired-state changes",
		Commands: []*cli.Command{
			{Name: "list", Usage: "list Change requests", Action: actionChangeList, Flags: outputFlags()},
			{Name: "show", Usage: "show a Change request", ArgsUsage: "<change-id>", Action: actionChangeShow, Flags: outputFlags()},
			{Name: "regenerate", Usage: "derive a canonical replacement for a legacy Change request", ArgsUsage: "<legacy-change-id>", Action: actionChangeRegenerate, Flags: outputFlags()},
			{Name: "watch", Usage: "watch a Change request", ArgsUsage: "<change-id>", Action: actionChangeWatch, Flags: append(outputFlags(), &cli.DurationFlag{Name: "interval", Value: 2 * time.Second}, &cli.DurationFlag{Name: "timeout"})},
			{Name: "authorize", Usage: "authorize a bounded rollout", ArgsUsage: "<change-id>", Action: actionChangeAuthorize, Flags: append(outputFlags(), &cli.IntFlag{Name: "attempt-limit", Value: 1}, &cli.IntFlag{Name: "max-concurrency", Value: 1}, &cli.StringFlag{Name: "justification", Required: true})},
			{Name: "pause", Usage: "pause new execution leases", ArgsUsage: "<change-id>", Action: actionChangeLifecycle("pause"), Flags: outputFlags()},
			{Name: "resume", Usage: "resume an authorized rollout", ArgsUsage: "<change-id>", Action: actionChangeLifecycle("resume"), Flags: outputFlags()},
			{Name: "revoke", Usage: "revoke a rollout", ArgsUsage: "<change-id>", Action: actionChangeLifecycle("revoke"), Flags: outputFlags()},
			{Name: "baseline-promote", Usage: "promote one verified resource to Fleet baseline", ArgsUsage: "<change-id>", Action: actionChangeBaselinePromote, Flags: append(outputFlags(), &cli.StringFlag{Name: "resource", Required: true}, &cli.BoolFlag{Name: "acknowledge-exceptions"})},
			{Name: "baseline-adopt", Usage: "derive and create one reviewed baseline-adoption request", Action: actionChangeBaselineAdopt, Flags: append(outputFlags(), &cli.StringFlag{Name: "fleet", Required: true})},
		},
	}
}

func changeClient(c *cli.Command, operation string) (*admin.Client, error) {
	settings, err := resolveSettings(c)
	if err != nil {
		return nil, exitErr(2, "%s: %v", operation, err)
	}
	if err := requireOperatorCLI(settings, operation); err != nil {
		return nil, err
	}
	client, err := newAdminClient(settings)
	if err != nil {
		return nil, exitErr(1, "%s: %v", operation, err)
	}
	return client, nil
}

func changeID(c *cli.Command) (string, error) {
	id := strings.TrimSpace(c.Args().First())
	if id == "" {
		return "", exitErr(2, "change id required")
	}
	return id, nil
}

func actionChangeList(_ context.Context, c *cli.Command) error {
	client, err := changeClient(c, "change list")
	if err != nil {
		return err
	}
	requests, err := client.ListChangeRequests()
	if err != nil {
		return apiErr(c, "change list", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(requests)
	}
	for _, request := range requests {
		printChangeSummary(request)
	}
	return nil
}

func actionChangeShow(_ context.Context, c *cli.Command) error {
	return showChange(c)
}

func showChange(c *cli.Command) error {
	id, err := changeID(c)
	if err != nil {
		return err
	}
	client, err := changeClient(c, "change show")
	if err != nil {
		return err
	}
	request, err := client.GetChangeRequest(id)
	if err != nil {
		return apiErr(c, "change show", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(request)
	}
	printChangeDetail(request)
	return nil
}

func actionChangeWatch(ctx context.Context, c *cli.Command) error {
	timeout := c.Duration("timeout")
	if timeout <= 0 {
		return showChange(c)
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := showChange(c); err != nil {
			return err
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.Duration("interval")):
		}
	}
}

func actionChangeAuthorize(_ context.Context, c *cli.Command) error {
	id, err := changeID(c)
	if err != nil {
		return err
	}
	client, err := changeClient(c, "change authorize")
	if err != nil {
		return err
	}
	authorization, err := client.AuthorizeChangeRequest(id, admin.RolloutSpec{AttemptLimit: c.Int("attempt-limit"), MaxConcurrency: c.Int("max-concurrency")}, c.String("justification"))
	if err != nil {
		return apiErr(c, "change authorize", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(authorization)
	}
	fmt.Printf("authorized: %s\nvalid_until: %s\n", authorization.ID, authorization.ValidUntil.UTC().Format(time.RFC3339))
	return nil
}

func actionChangeRegenerate(_ context.Context, c *cli.Command) error {
	id, err := changeID(c)
	if err != nil {
		return err
	}
	client, err := changeClient(c, "change regenerate")
	if err != nil {
		return err
	}
	result, err := client.RegenerateChangeRequest(id)
	if err != nil {
		return apiErr(c, "change regenerate", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(result)
	}
	fmt.Printf("legacy: %s  enforcement=%s\n", result.LegacyRequest.ID, result.LegacyRequest.LegacyMigration.Enforcement)
	fmt.Printf("replacement: %s  state=%s\n", result.ReplacementRequest.ID, result.ReplacementRequest.AuthorizationState)
	for _, resource := range result.Comparison.Resources {
		fmt.Printf("  - %s  %s  canonical_hash=%s\n", resource.Address, resource.Status, resource.CanonicalHash)
	}
	return nil
}

func actionChangeLifecycle(action string) cli.ActionFunc {
	return func(_ context.Context, c *cli.Command) error {
		id, err := changeID(c)
		if err != nil {
			return err
		}
		client, err := changeClient(c, "change "+action)
		if err != nil {
			return err
		}
		request, err := client.ChangeRequestLifecycle(id, action)
		if err != nil {
			return apiErr(c, "change "+action, err)
		}
		if resolveFormat(c) == formatJSON {
			return encodeJSON(request)
		}
		printChangeSummary(request)
		return nil
	}
}

func actionChangeBaselinePromote(_ context.Context, c *cli.Command) error {
	id, err := changeID(c)
	if err != nil {
		return err
	}
	client, err := changeClient(c, "change baseline-promote")
	if err != nil {
		return err
	}
	baseline, err := client.PromoteChangeBaseline(id, c.String("resource"), c.Bool("acknowledge-exceptions"))
	if err != nil {
		return apiErr(c, "change baseline-promote", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(baseline)
	}
	fmt.Printf("baseline: %s\nresource: %s\nhash: %s\n", baseline.ID, baseline.ResourceAddress, baseline.DesiredHash)
	return nil
}

func actionChangeBaselineAdopt(_ context.Context, c *cli.Command) error {
	client, err := changeClient(c, "change baseline-adopt")
	if err != nil {
		return err
	}
	request, err := client.CreateBaselineAdoption(c.String("fleet"))
	if err != nil {
		return apiErr(c, "change baseline-adopt", err)
	}
	if resolveFormat(c) == formatJSON {
		return encodeJSON(request)
	}
	printChangeDetail(request)
	return nil
}

func printChangeSummary(request admin.ChangeRequest) {
	state := string(request.AuthorizationState)
	if request.LegacyMigration != nil {
		state = request.LegacyMigration.Enforcement
	}
	fmt.Printf("%s  %s  %s  %s  %d targets\n", request.ID, request.Fleet, request.Risk, state, len(request.FrozenTargets))
}

func printChangeDetail(request admin.ChangeRequest) {
	fmt.Printf("change: %s\nfleet: %s\nrelease_ref: %s\ngroup: %s\nrisk: %s\nstate: %s\n", request.ID, request.Fleet, request.ReleaseRef, request.AuthorizationGroup, request.Risk, request.AuthorizationState)
	if request.LegacyMigration != nil {
		fmt.Printf("enforcement: %s\nmigration_reason: %s\nreplacement: %s\n", request.LegacyMigration.Enforcement, request.LegacyMigration.Reason, request.LegacyMigration.Replacement)
		if request.LegacyMigration.ReplacementChangeRequestID != "" {
			fmt.Printf("replacement_change: %s\n", request.LegacyMigration.ReplacementChangeRequestID)
		}
	}
	fmt.Println("resources:")
	for _, resource := range request.Resources {
		fmt.Printf("  - %s  %s  %s\n", resource.Address, resource.DesiredHash, resource.Risk)
		for _, effect := range resource.PredictedEffects {
			fmt.Printf("      effect: %s\n", effect.String())
		}
	}
	fmt.Println("targets:")
	for _, target := range request.FrozenTargets {
		fmt.Printf("  - %s  compatible=%t  preflight_ready=%t\n", target.EndpointID, target.Compatible, target.PreflightReady)
	}
}
