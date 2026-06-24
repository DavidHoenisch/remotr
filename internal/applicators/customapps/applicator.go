package customapps

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

type Applicator struct {
	Package models.Package
	Facts   facts.Facts
	Exec    executil.Runner
	URLs    apppackages.URLResolver
	WorkDir string
}

func New(pkg models.Package, f facts.Facts, exec executil.Runner, urls apppackages.URLResolver) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Package: pkg, Facts: f, Exec: exec, URLs: urls}
}

func (a *Applicator) Name() string { return "remotr:" + a.Package.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("remotr package %s@%s", a.Package.Name, a.Package.Version)
}

func (a *Applicator) present() bool {
	return a.Package.Present
}

func (a *Applicator) versionFile() string {
	return apppackages.DefaultVersionFile(a.Package.Name)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	if !a.present() {
		if _, err := os.Stat(a.versionFile()); os.IsNotExist(err) {
			return nil, true
		}
		return nil, false
	}
	got, err := os.ReadFile(a.versionFile()) // #nosec G304 -- fixed path under /var/lib/remotr
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(a.Package.Version) {
		return string(got), false
	}
	return string(got), true
}

func (a *Applicator) Apply(ctx context.Context) error {
	if !a.present() {
		return a.uninstall()
	}
	if _, met := a.State(ctx); met {
		return appErr.ErrStateAlreadyMet
	}
	if a.URLs == nil {
		return fmt.Errorf("remotr package %q: package URL resolver unavailable", a.Package.Name)
	}

	grant, err := a.URLs.DownloadURL(ctx, a.Package.Name, a.Package.Version)
	if err != nil {
		return fmt.Errorf("remotr package %q: download url: %w", a.Package.Name, err)
	}
	data, err := a.fetch(ctx, grant.URL)
	if err != nil {
		return err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != strings.ToLower(grant.SHA256) {
		return fmt.Errorf("remotr package %q: zip checksum mismatch", a.Package.Name)
	}

	workDir, cleanup, err := a.makeWorkDir()
	if err != nil {
		return err
	}
	defer cleanup()

	extractDir := filepath.Join(workDir, "extract")
	if err := extractZip(data, extractDir); err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(extractDir, apppackages.ManifestName)) // #nosec G304
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest apppackages.Manifest
	if err := yaml.Unmarshal(manifestRaw, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if err := apppackages.ValidateManifest(manifest); err != nil {
		return err
	}
	if manifest.Name != a.Package.Name || manifest.Version != a.Package.Version {
		return fmt.Errorf("manifest name/version mismatch")
	}

	if err := a.install(ctx, extractDir, manifest); err != nil {
		return err
	}
	versionFile := a.versionFile()
	if err := os.MkdirAll(filepath.Dir(versionFile), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(versionFile, []byte(a.Package.Version), 0o644); err != nil { // #nosec G703
		return err
	}
	if len(manifest.Check.Command) > 0 {
		return a.runCheck(manifest)
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error {
	_ = os.Remove(a.versionFile())
	return nil
}

func (a *Applicator) uninstall() error {
	versionFile := a.versionFile()
	if _, err := os.Stat(versionFile); os.IsNotExist(err) {
		return appErr.ErrStateAlreadyMet
	}
	_ = os.Remove(versionFile)
	return nil
}

func (a *Applicator) install(ctx context.Context, extractDir string, manifest apppackages.Manifest) error {
	switch manifest.Install.Mode {
	case "binary":
		return a.installBinary(extractDir, manifest)
	case "script":
		return a.runScript(ctx, extractDir, manifest.Install.Script)
	case "build":
		for _, step := range manifest.Install.Build {
			if err := a.runStep(ctx, extractDir, step); err != nil {
				return err
			}
		}
		script := manifest.Install.Script
		if len(script) == 0 {
			script = []string{"./install.sh"}
		}
		return a.runScript(ctx, extractDir, script)
	default:
		return fmt.Errorf("unsupported install mode %q", manifest.Install.Mode)
	}
}

func (a *Applicator) installBinary(extractDir string, manifest apppackages.Manifest) error {
	for _, f := range manifest.Install.Files {
		if f.Arch != "" && f.Arch != a.Facts.Arch {
			continue
		}
		src := filepath.Join(extractDir, filepath.FromSlash(f.Src))
		data, err := os.ReadFile(src) // #nosec G304 -- path under extract dir
		if err != nil {
			return fmt.Errorf("read %s: %w", f.Src, err)
		}
		if err := os.MkdirAll(filepath.Dir(f.Dest), 0o750); err != nil {
			return err
		}
		mode := os.FileMode(0o755)
		if m := strings.TrimSpace(f.Mode); m != "" {
			parsed, err := strconv.ParseUint(m, 8, 32)
			if err != nil {
				return fmt.Errorf("invalid mode %q: %w", f.Mode, err)
			}
			mode = os.FileMode(parsed)
		}
		if err := os.WriteFile(f.Dest, data, mode); err != nil { // #nosec G703
			return fmt.Errorf("write %s: %w", f.Dest, err)
		}
	}
	return nil
}

func (a *Applicator) runScript(ctx context.Context, dir string, argv []string) error {
	return a.runStep(ctx, dir, argv)
}

func (a *Applicator) runStep(ctx context.Context, dir string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := append([]string(nil), argv...)
	if strings.HasPrefix(cmd[0], "./") {
		cmd[0] = filepath.Join(dir, cmd[0][2:])
	}
	_, _, err := a.Exec.Run(cmd[0], cmd[1:]...)
	_ = ctx
	return err
}

func (a *Applicator) runCheck(manifest apppackages.Manifest) error {
	cmd := manifest.Check.Command
	out, _, err := a.Exec.Run(cmd[0], cmd[1:]...)
	if err != nil {
		return fmt.Errorf("check command failed: %w", err)
	}
	if !strings.Contains(string(out), manifest.Check.Expect) {
		return fmt.Errorf("check expect %q not found in output", manifest.Check.Expect)
	}
	return nil
}

func (a *Applicator) fetch(ctx context.Context, url string) ([]byte, error) {
	out, _, err := a.Exec.Run("curl", "-fsSL", url)
	if err == nil {
		return out, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (a *Applicator) makeWorkDir() (string, func(), error) {
	if a.WorkDir != "" {
		return a.WorkDir, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "remotr-customapp-*")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func extractZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return err
	}
	destClean := filepath.Clean(dest)
	for _, f := range zr.File {
		target := filepath.Join(dest, filepath.FromSlash(f.Name))
		targetClean := filepath.Clean(target)
		if targetClean != destClean && !strings.HasPrefix(targetClean, destClean+string(os.PathSeparator)) {
			return fmt.Errorf("zip path escapes extract dir: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, body, f.Mode()); err != nil { // #nosec G703
			return err
		}
	}
	return nil
}
