package engine

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// ServiceActionError is the safe, structured failure returned by the
// controlled systemd activation boundary. Raw stdout/stderr is never retained.
type ServiceActionError struct {
	Provider   string
	Unit       string
	Operation  string
	ExitStatus int
	Diagnostic executor.RedactedSummary
}

func (e *ServiceActionError) Error() string {
	return fmt.Sprintf("%s service action failed: unit=%q operation=%q exit_status=%d diagnostic=%q", e.Provider, e.Unit, e.Operation, e.ExitStatus, e.Diagnostic)
}

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
		case executor.ActivationTryRestart:
			args = []string{"try-restart", signal.Target}
		case executor.ActivationRestart:
			args = []string{"restart", signal.Target}
		case executor.ActivationTrustStoreRefresh:
			switch signal.Target {
			case "debian":
				_, stderr, err := a.runner.Run("update-ca-certificates")
				if err != nil {
					return &ServiceActionError{Provider: "ca-trust", Unit: signal.Target, Operation: "refresh", ExitStatus: -1, Diagnostic: redactedCommandDiagnostic(stderr)}
				}
				continue
			case "arch":
				_, stderr, err := a.runner.Run("trust", "extract-compat")
				if err != nil {
					return &ServiceActionError{Provider: "ca-trust", Unit: signal.Target, Operation: "refresh", ExitStatus: -1, Diagnostic: redactedCommandDiagnostic(stderr)}
				}
				continue
			default:
				return fmt.Errorf("unsupported trust-store refresh target %q", signal.Target)
			}
		case executor.ActivationLogoutRequired, executor.ActivationApplicationRestart, executor.ActivationNextBoot, executor.ActivationRebootRequired:
			// These activations deliberately never terminate a session or reboot
			// incidentally. Their visibility is retained in ApplyResult instead.
			continue
		default:
			return fmt.Errorf("unsupported activation %q", signal.Kind)
		}
		_, stderr, err := a.runner.Run("systemctl", args...)
		if err != nil {
			exitStatus := -1
			if coded, ok := err.(interface{ ExitCode() int }); ok {
				exitStatus = coded.ExitCode()
			}
			diagnostic := executor.RedactedSummary("systemctl returned no safe diagnostic output")
			if len(stderr) > 0 {
				diagnostic = "systemctl stderr was redacted"
			}
			return &ServiceActionError{
				Provider: "systemd", Unit: signal.Target, Operation: args[0], ExitStatus: exitStatus, Diagnostic: diagnostic,
			}
		}
	}
	return nil
}

func redactedCommandDiagnostic(stderr []byte) executor.RedactedSummary {
	if len(stderr) > 0 {
		return "command stderr was redacted"
	}
	return "command returned no safe diagnostic output"
}
