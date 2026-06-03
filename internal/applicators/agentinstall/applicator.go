package agentinstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type Applicator struct {
	Resource models.AgentInstallResource
	Exec     executil.Runner
	// WorkDir, when set, is used instead of MkdirTemp (tests only).
	WorkDir string
}

func New(r models.AgentInstallResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Resource: r, Exec: exec}
}

func (a *Applicator) Name() string { return "agentInstall:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return "agent install " + a.Resource.Name
}

func (a *Applicator) present() bool {
	if a.Resource.Present != nil {
		return *a.Resource.Present
	}
	return true
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	if !a.present() {
		return nil, true
	}
	return nil, a.isRunning()
}

func (a *Applicator) isRunning() bool {
	pat := strings.TrimSpace(a.Resource.RunningCheck.Process)
	if pat == "" {
		return false
	}
	_, _, err := a.Exec.Run("pgrep", "-f", pat)
	return err == nil
}

func (a *Applicator) Apply(_ context.Context) error {
	if !a.present() {
		return appErr.ErrStateAlreadyMet
	}
	if a.isRunning() {
		return appErr.ErrStateAlreadyMet
	}
	token, err := secrets.ReadFileRef(a.Resource.EnrollmentTokenSecret)
	if err != nil {
		return err
	}
	version := strings.TrimSpace(a.Resource.Version)
	if version == "" {
		return fmt.Errorf("agentInstall %q: version required", a.Resource.Name)
	}
	artifactURL := expandVersion(a.Resource.ArtifactURL, version)
	extractDir := expandVersion(a.Resource.ExtractDir, version)
	if artifactURL == "" || extractDir == "" {
		return fmt.Errorf("agentInstall %q: artifactURL and extractDir required", a.Resource.Name)
	}
	tmp := strings.TrimSpace(a.WorkDir)
	cleanup := false
	if tmp == "" {
		var err error
		tmp, err = os.MkdirTemp("", "remotr-agent-*")
		if err != nil {
			return err
		}
		cleanup = true
	}
	if cleanup {
		defer os.RemoveAll(tmp) // #nosec G104 -- best-effort cleanup
	}

	return a.applyInWorkDir(tmp, artifactURL, extractDir, token)
}

func (a *Applicator) applyInWorkDir(tmp, artifactURL, extractDir, token string) error {
	tarball := filepath.Base(artifactURL)
	tarPath := filepath.Join(tmp, tarball)
	if _, _, err := a.Exec.Run("curl", "-fsSL", "-o", tarPath, artifactURL); err != nil {
		return fmt.Errorf("download %s: %w", artifactURL, err)
	}
	if _, _, err := a.Exec.Run("tar", "-xzf", tarPath, "-C", tmp); err != nil {
		return fmt.Errorf("extract %s: %w", tarPath, err)
	}
	installDir := filepath.Join(tmp, extractDir)
	binary := strings.TrimSpace(a.Resource.InstallBinary)
	if binary == "" {
		binary = "elastic-agent"
	}
	fleetURL := strings.TrimSpace(a.Resource.FleetURL)
	if fleetURL == "" {
		return fmt.Errorf("agentInstall %q: fleetURL required", a.Resource.Name)
	}
	if strings.ContainsAny(binary, " ;&|$`\"'\\") {
		return fmt.Errorf("agentInstall %q: invalid installBinary", a.Resource.Name)
	}
	script := fmt.Sprintf(
		"set -euo pipefail; cd %q; ./%s install --url=%q --enrollment-token=%q",
		installDir, binary, fleetURL, token,
	)
	if _, _, err := a.Exec.Run("bash", "-c", script); err != nil {
		return fmt.Errorf("install %s: %w", a.Resource.Name, err)
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func expandVersion(s, version string) string {
	return strings.ReplaceAll(s, "${version}", version)
}
