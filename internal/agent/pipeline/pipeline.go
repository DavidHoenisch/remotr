package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Run parses artifact YAML, resolves, checks, and optionally applies.
func Run(ctx context.Context, artifactYAML []byte, policy engine.Policy, exec executil.Runner) (Result, error) {
	out, eng, err := check(ctx, artifactYAML, exec)
	if err != nil {
		return out, err
	}
	if out.Drift.InCompliance {
		return out, nil
	}
	slog.Info("drift detected", "count", len(out.Drift.Items))
	for _, item := range out.Drift.Items {
		slog.Info("drift", "address", item.Address, "description", item.Description)
	}

	if policy == "" {
		policy = engine.PolicyAuto
	}
	result := eng.ApplyAll(ctx, policy)
	if result.Failed != nil {
		out.ApplyFailure = result.Failed
		return out, fmt.Errorf("apply failed at %s: %w", result.Failed.Address, result.Failed.Err)
	}
	if len(result.Applied) > 0 {
		slog.Info("apply complete", "applied", result.Applied)
	}
	if len(result.Skipped) > 0 {
		slog.Info("apply skipped (report policy)", "skipped", result.Skipped)
	}
	return out, nil
}

// Check parses artifact YAML, resolves, and checks without applying changes.
func Check(ctx context.Context, artifactYAML []byte, exec executil.Runner) (Result, error) {
	out, _, err := check(ctx, artifactYAML, exec)
	return out, err
}

func check(ctx context.Context, artifactYAML []byte, exec executil.Runner) (Result, *engine.Engine, error) {
	state, err := models.ParseState(bytes.NewReader(artifactYAML))
	if err != nil {
		return Result{}, nil, fmt.Errorf("parse artifact: %w", err)
	}

	f, err := facts.Read()
	if err != nil {
		return Result{}, nil, fmt.Errorf("read facts: %w", err)
	}

	labels := map[string]string{
		"distro": string(f.Distro),
		"arch":   string(f.Arch),
	}

	resolved := resolve.Resolve(state, f)
	eng, err := engine.New(resolved, f, exec)
	if err != nil {
		return Result{Labels: labels}, nil, fmt.Errorf("build engine: %w", err)
	}

	drift := eng.CheckAll(ctx)
	out := Result{Labels: labels, Drift: drift}
	if drift.InCompliance {
		slog.Info("check complete", "status", "in compliance", "resources", eng.NodeCount())
	}
	return out, eng, nil
}

// PolicyFromResponse maps sync remediation policy to engine policy.
func PolicyFromResponse(raw string) engine.Policy {
	switch raw {
	case "report":
		return engine.PolicyReport
	default:
		return engine.PolicyAuto
	}
}
