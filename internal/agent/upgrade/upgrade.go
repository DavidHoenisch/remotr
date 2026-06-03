package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/executil"
)

// Instruction is returned on sync when the server taints the endpoint for upgrade.
type Instruction struct {
	Version    string
	GitHubRepo string
}

// Options configures a self-upgrade run.
type Options struct {
	CurrentVersion string
	BinDir         string
	GitHubRepo     string
	Exec           executil.Runner
}

// Needed reports whether instruction targets a different version than current.
func Needed(inst Instruction, current string) bool {
	if strings.TrimSpace(inst.Version) == "" {
		return false
	}
	return !agentversion.Match(inst.Version, current)
}

// Apply downloads and installs remotr-agent, then restarts remotr-agent.service.
func Apply(inst Instruction, opt Options) error {
	if opt.Exec == nil {
		opt.Exec = executil.OSRunner{}
	}
	ver, err := agentversion.Normalize(inst.Version)
	if err != nil {
		return err
	}
	repo := strings.TrimSpace(inst.GitHubRepo)
	if repo == "" {
		repo = strings.TrimSpace(opt.GitHubRepo)
	}
	if repo == "" {
		repo = "DavidHoenisch/remotr"
	}
	arch, err := detectArch()
	if err != nil {
		return err
	}
	tag := ver
	version := strings.TrimPrefix(ver, "v")
	asset := fmt.Sprintf("remotr-agent_%s_linux_%s.tar.gz", version, arch)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)
	url := base + "/" + asset

	tmp, err := os.MkdirTemp("", "remotr-agent-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // #nosec G104

	tarPath := filepath.Join(tmp, asset)
	if _, _, err := opt.Exec.Run("curl", "-fsSL", "-o", tarPath, url); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if _, _, err := opt.Exec.Run("tar", "-xzf", tarPath, "-C", tmp); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	src := filepath.Join(tmp, "remotr-agent")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("archive missing remotr-agent: %w", err)
	}
	binDir := strings.TrimSpace(opt.BinDir)
	if binDir == "" {
		binDir = "/usr/local/bin"
	}
	dest := filepath.Join(binDir, "remotr-agent")
	if err := installBinary(src, dest); err != nil {
		return err
	}
	if _, _, err := opt.Exec.Run("systemctl", "restart", "remotr-agent.service"); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}
	return nil
}

// installBinary replaces dest without opening the running executable for write
// (which returns ETXTBSY on Linux). Write a staging file, then rename over dest;
// the old inode stays mapped until the process exits.
func installBinary(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src) // #nosec G304
	if err != nil {
		return err
	}
	staging := dest + ".new"
	if err := os.WriteFile(staging, data, 0o755); err != nil { // #nosec G306 G703
		return fmt.Errorf("stage binary: %w", err)
	}
	if err := os.Rename(staging, dest); err != nil {
		_ = os.Remove(staging)
		return fmt.Errorf("install binary: %w", err)
	}
	return nil
}

func detectArch() (string, error) {
	machine, _, err := executil.OSRunner{}.Run("uname", "-m")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(string(machine)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", machine)
	}
}
