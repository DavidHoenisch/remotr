package engine

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// OS-IUP-008: policy activation requirements remain report-only at the
// controlled activation boundary; they never terminate sessions or browsers.
func TestPolicyActivationRequirementsDoNotTerminateSessionsOrApplications(t *testing.T) {
	runner := &executil.MockRunner{}
	err := (systemActivator{runner: runner}).Activate(context.Background(), []executor.ActivationSignal{
		{Kind: executor.ActivationLogoutRequired},
		{Kind: executor.ActivationApplicationRestart, Target: "firefox"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("report-only policy activation executed commands: %+v", runner.Calls)
	}
}
