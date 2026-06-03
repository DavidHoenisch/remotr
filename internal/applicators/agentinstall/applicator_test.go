package agentinstall_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/agentinstall"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func mockKey(name string, args ...string) string {
	return fmt.Sprintf("%s %v", name, args)
}

func TestApplicator_State_running(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			mockKey("pgrep", "-f", "elastic-agent"): {Err: nil},
		},
	}
	present := true
	a := agentinstall.New(models.AgentInstallResource{
		Name:    "elastic-agent",
		Present: &present,
		RunningCheck: models.AgentRunningCheck{Process: "elastic-agent"},
	}, mock)
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected running")
	}
}

func TestApplicator_State_notRunning(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			mockKey("pgrep", "-f", "elastic-agent"): {Err: os.ErrNotExist},
		},
	}
	a := agentinstall.New(models.AgentInstallResource{
		Name:         "elastic-agent",
		RunningCheck: models.AgentRunningCheck{Process: "elastic-agent"},
	}, mock)
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift")
	}
}

func TestApplicator_Apply_installs(t *testing.T) {
	work := t.TempDir()
	tokenPath := filepath.Join(work, "token")
	if err := os.WriteFile(tokenPath, []byte("tok123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	artifactURL := "https://example.com/agent.tar.gz"
	tarPath := filepath.Join(work, "agent.tar.gz")
	installDir := filepath.Join(work, "elastic-agent-1.0-linux-x86_64")
	script := fmt.Sprintf(
		"set -euo pipefail; cd %q; ./elastic-agent install --url=%q --enrollment-token=%q",
		installDir, "https://fleet.example", "tok123",
	)
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			mockKey("pgrep", "-f", "elastic-agent"):                          {Err: os.ErrNotExist},
			mockKey("curl", "-fsSL", "-o", tarPath, artifactURL):             {},
			mockKey("tar", "-xzf", tarPath, "-C", work):                      {},
			mockKey("bash", "-c", script):                                    {},
		},
	}
	a := agentinstall.New(models.AgentInstallResource{
		Name:                  "elastic-agent",
		Version:               "1.0",
		ArtifactURL:           artifactURL,
		ExtractDir:            "elastic-agent-${version}-linux-x86_64",
		FleetURL:              "https://fleet.example",
		EnrollmentTokenSecret: "file:" + tokenPath,
		RunningCheck:          models.AgentRunningCheck{Process: "elastic-agent"},
	}, mock)
	a.WorkDir = work
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mock.Calls) != 4 {
		t.Fatalf("calls = %d, want 4 (%v)", len(mock.Calls), mock.Calls)
	}
}

func TestApplicator_Apply_missingToken(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			mockKey("pgrep", "-f", "elastic-agent"): {Err: os.ErrNotExist},
		},
	}
	a := agentinstall.New(models.AgentInstallResource{
		Name:                  "elastic-agent",
		Version:               "1.0",
		ArtifactURL:           "https://example.com/a.tar.gz",
		ExtractDir:            "dir",
		FleetURL:              "https://fleet.example",
		EnrollmentTokenSecret: "file:/nonexistent/token",
		RunningCheck:          models.AgentRunningCheck{Process: "elastic-agent"},
	}, mock)
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplicator_Apply_alreadyRunning(t *testing.T) {
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			mockKey("pgrep", "-f", "elastic-agent"): {},
		},
	}
	a := agentinstall.New(models.AgentInstallResource{
		Name:         "elastic-agent",
		RunningCheck: models.AgentRunningCheck{Process: "elastic-agent"},
	}, mock)
	if err := a.Apply(context.Background()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("expected ErrStateAlreadyMet, got %v", err)
	}
}
