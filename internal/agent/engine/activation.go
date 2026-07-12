package engine

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

type systemActivator struct {
	runner executil.Runner
}

func (a systemActivator) Activate(ctx context.Context, signals []executor.ActivationSignal) error {
	for _, signal := range signals {
		if err := ctx.Err(); err != nil {
			return err
		}
		var args []string
		switch signal.Kind {
		case executor.ActivationDaemonReload:
			args = []string{"daemon-reload"}
		case executor.ActivationReload:
			args = []string{"reload", signal.Target}
		case executor.ActivationRestart:
			args = []string{"restart", signal.Target}
		case executor.ActivationLogoutRequired, executor.ActivationNextBoot, executor.ActivationRebootRequired:
			// These activations deliberately never terminate a session or reboot
			// incidentally. Their visibility is retained in ApplyResult instead.
			continue
		default:
			return fmt.Errorf("unsupported activation %q", signal.Kind)
		}
		if _, _, err := a.runner.Run("systemctl", args...); err != nil {
			return fmt.Errorf("activation %q: %w", signal.Kind, err)
		}
	}
	return nil
}
