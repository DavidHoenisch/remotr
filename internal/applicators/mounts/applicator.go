package mounts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource        models.MountResource
	Runner          executil.Runner
	FstabPath       string
	MountInfoPath   string
	FilesystemsPath string
	StateDir        string
}

func New(resource models.MountResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner, FstabPath: "/etc/fstab", MountInfoPath: "/proc/self/mountinfo", FilesystemsPath: "/proc/filesystems", StateDir: "/var/lib/remotr"}
}

func (a *Applicator) Name() string        { return "mount:" + a.Resource.Name }
func (a *Applicator) Description() string { return "mount " + a.Resource.Target }
func (a *Applicator) State(ctx context.Context) (any, bool) {
	c := a.Check(ctx)
	return c.ObservedSummary, c.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("mount " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return failed(desired, err)
	}
	if err := a.preflight(); err != nil {
		return failed(desired, err)
	}
	if a.Resource.Mounted != nil {
		mounted, err := a.mountState()
		if err != nil {
			return failed(desired, err)
		}
		if mounted.matches(a.Resource) != *a.Resource.Mounted {
			return drifted(desired, "runtime mount state differs")
		}
	}
	if want, managed := a.Resource.DesiredPersistent(); managed {
		body, err := os.ReadFile(a.FstabPath)
		if err != nil && !os.IsNotExist(err) {
			return failed(desired, err)
		}
		has := a.ownedEntry(string(body)) != ""
		if has != want || (want && a.ownedEntry(string(body)) != a.entry()) {
			return drifted(desired, "owned fstab entry differs")
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.preflight(); err != nil {
		return err
	}
	changed := false
	if want, managed := a.Resource.DesiredPersistent(); managed {
		body, err := os.ReadFile(a.FstabPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		next := a.withOwnedEntry(string(body), want)
		if next != string(body) {
			if err := writeAtomic(a.FstabPath, []byte(next), 0o644); err != nil {
				return err
			}
			changed = true
		}
	}
	if a.Resource.Mounted != nil {
		mounted, err := a.mountState()
		if err != nil {
			return err
		}
		if mounted.matches(a.Resource) != *a.Resource.Mounted {
			if *a.Resource.Mounted {
				if err := a.mount(); err != nil {
					return err
				}
			} else if err := a.unmount(); err != nil {
				return err
			}
			changed = true
		}
	}
	if !changed {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}
func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
}
func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) preflight() error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(a.FstabPath) || !filepath.IsAbs(a.MountInfoPath) || !filepath.IsAbs(a.FilesystemsPath) || !filepath.IsAbs(a.StateDir) {
		return errors.New("mount provider paths must be absolute")
	}
	wantPersistent, managesPersistent := a.Resource.DesiredPersistent()
	wantRuntime := a.Resource.Mounted != nil && *a.Resource.Mounted
	if a.Resource.Mounted != nil || (managesPersistent && wantPersistent) {
		if isProtected(a.Resource.Target, a.StateDir) {
			return fmt.Errorf("mount target %q would hide or detach Remotr state directory", a.Resource.Target)
		}
		if filepath.IsAbs(a.Resource.Source) && pathsOverlap(a.Resource.Source, a.StateDir) {
			return fmt.Errorf("protected mount source %q overlaps Remotr state directory", a.Resource.Source)
		}
	}
	if wantRuntime || (managesPersistent && wantPersistent) {
		if info, err := os.Stat(a.Resource.Target); err != nil || !info.IsDir() {
			return fmt.Errorf("mount target %q is not a directory", a.Resource.Target)
		}
		if err := a.checkSourceAndFilesystem(); err != nil {
			return err
		}
	}
	return nil
}
func isProtected(target, state string) bool {
	target, state = filepath.Clean(target), filepath.Clean(state)
	return target == state || strings.HasPrefix(state, target+string(filepath.Separator))
}

func pathsOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	separator := string(filepath.Separator)
	return left == right || strings.HasPrefix(left, right+separator) || strings.HasPrefix(right, left+separator)
}

type observedMount struct {
	mounted            bool
	source, filesystem string
	options            map[string]bool
}

func (m observedMount) matches(resource models.MountResource) bool {
	if !m.mounted {
		return false
	}
	if m.filesystem != "" && m.filesystem != resource.FilesystemType {
		return false
	}
	if m.source != "" && m.source != resource.Source {
		return false
	}
	for _, option := range resource.NormalizedOptions() {
		if !m.options[option] {
			return false
		}
	}
	return true
}

func (a *Applicator) mountState() (observedMount, error) {
	body, err := os.ReadFile(a.MountInfoPath)
	if err != nil {
		return observedMount{}, err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[4] == a.Resource.Target {
			state := observedMount{mounted: true, options: map[string]bool{}}
			for i, value := range fields {
				if value == "-" && len(fields) > i+3 {
					state.filesystem, state.source = fields[i+1], fields[i+2]
					for _, option := range strings.Split(fields[i+3], ",") {
						state.options[option] = true
					}
					break
				}
			}
			return state, nil
		}
	}
	return observedMount{}, nil
}
func (a *Applicator) mount() error {
	args := []string{"-t", a.Resource.FilesystemType}
	if options := a.Resource.NormalizedOptions(); len(options) > 0 {
		args = append(args, "-o", strings.Join(options, ","))
	}
	args = append(args, a.Resource.Source, a.Resource.Target)
	if _, stderr, err := a.Runner.Run("mount", args...); err != nil {
		return fmt.Errorf("mount %s: %s: %w", a.Resource.Target, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}
func (a *Applicator) unmount() error {
	args := []string{}
	switch a.Resource.UnmountMode {
	case models.UnmountLazy:
		args = append(args, "-l")
	case models.UnmountForce:
		args = append(args, "-f")
	}
	args = append(args, a.Resource.Target)
	if _, stderr, err := a.Runner.Run("umount", args...); err != nil {
		return fmt.Errorf("unmount %s: %s: %w", a.Resource.Target, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}
func (a *Applicator) entry() string {
	options := a.Resource.NormalizedOptions()
	if len(options) == 0 {
		options = []string{"defaults"}
	}
	return a.Resource.Source + " " + a.Resource.Target + " " + a.Resource.FilesystemType + " " + strings.Join(options, ",") + fmt.Sprintf(" %d %d # remotr:%s\n", a.Resource.Dump, a.Resource.Pass, a.Resource.Name)
}
func (a *Applicator) ownedEntry(body string) string {
	for _, line := range strings.SplitAfter(body, "\n") {
		if strings.HasSuffix(strings.TrimSuffix(line, "\n"), "# remotr:"+a.Resource.Name) {
			return line
		}
	}
	return ""
}
func (a *Applicator) withOwnedEntry(body string, present bool) string {
	lines := strings.SplitAfter(body, "\n")
	kept := make([]string, 0, len(lines)+1)
	marker := "# remotr:" + a.Resource.Name
	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSuffix(line, "\n"), marker) {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "")
	if present {
		if result != "" && !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		result += a.entry()
	}
	return result
}
func (a *Applicator) checkSourceAndFilesystem() error {
	body, err := os.ReadFile(a.FilesystemsPath)
	if err != nil {
		return err
	}
	supported := false
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(strings.TrimPrefix(line, "nodev")) == a.Resource.FilesystemType {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("filesystem type %q is unsupported", a.Resource.FilesystemType)
	}
	if strings.HasPrefix(a.Resource.Source, "/") {
		if _, err := os.Stat(a.Resource.Source); err != nil {
			return fmt.Errorf("mount source %q is unavailable: %w", a.Resource.Source, err)
		}
	}
	return nil
}
func writeAtomic(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-mount-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func drifted(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}
func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
