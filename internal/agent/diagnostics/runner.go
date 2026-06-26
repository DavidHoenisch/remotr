package diagnostics

import (
	"context"
	"os/exec"
)

// CommandRunner executes fixed argv commands for read-only collectors.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// DefaultRunner is the production command runner.
var DefaultRunner CommandRunner = execRunner{}
