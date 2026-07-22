package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
	sumURL := base + "/checksums.txt"

	tmp, err := os.MkdirTemp("", "remotr-agent-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // #nosec G104

	tarPath := filepath.Join(tmp, asset)
	if _, _, err := opt.Exec.Run("curl", "-fsSL", "-o", tarPath, url); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	sumPath := filepath.Join(tmp, "checksums.txt")
	if _, _, err := opt.Exec.Run("curl", "-fsSL", "-o", sumPath, sumURL); err != nil {
		return fmt.Errorf("download checksum %s: %w", sumURL, err)
	}
	expected, err := readExpectedSHA256(sumPath, asset)
	if err != nil {
		return err
	}
	if err := verifySHA256(tarPath, expected); err != nil {
		return err
	}
	if err := extractAgentBinary(tarPath, tmp); err != nil {
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

func readExpectedSHA256(path, asset string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is generated inside a private temp dir.
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		sum := strings.TrimPrefix(fields[0], "sha256:")
		if len(fields) > 1 {
			name := strings.TrimPrefix(fields[1], "*")
			if filepath.Base(name) != asset {
				continue
			}
		}
		if len(sum) != sha256.Size*2 {
			return "", fmt.Errorf("invalid sha256 checksum length for %s", asset)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf("invalid sha256 checksum for %s: %w", asset, err)
		}
		return strings.ToLower(sum), nil
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func verifySHA256(path, expected string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- path is generated inside a private temp dir.
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
	}
	return nil
}

func extractAgentBinary(tarPath, destDir string) error {
	data, err := os.ReadFile(tarPath) // #nosec G304 -- path is generated inside a private temp dir.
	if err != nil {
		return err
	}
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var agentBinary []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		name := filepath.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
			return fmt.Errorf("unsafe archive path %q", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg || name != "remotr-agent" {
			continue
		}
		if agentBinary != nil {
			return fmt.Errorf("archive contains duplicate remotr-agent")
		}
		out, err := io.ReadAll(io.LimitReader(tr, 256<<20))
		if err != nil {
			return fmt.Errorf("read remotr-agent: %w", err)
		}
		if len(out) == 0 {
			return fmt.Errorf("archive remotr-agent is empty")
		}
		agentBinary = out
	}
	if agentBinary == nil {
		return fmt.Errorf("archive missing remotr-agent")
	}
	return os.WriteFile(filepath.Join(destDir, "remotr-agent"), agentBinary, 0o755) // #nosec G306
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
