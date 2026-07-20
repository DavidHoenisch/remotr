// Package kernelmodules manages Linux module state through modprobe and named
// modules-load.d/modprobe.d fragments.
package kernelmodules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator owns only the module's named Remotr fragments. The injectable
// paths make the provider contract testable against a controlled host boundary.
type Applicator struct {
	Resource       models.KernelModuleResource
	ProcModules    string
	ProcMountInfo  string
	SysModuleRoot  string
	SysRoot        string
	ModulesLoadDir string
	ModprobeDir    string
	Runner         executil.Runner
	HasModprobe    func() bool
}

func New(resource models.KernelModuleResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{
		Resource:       resource,
		ProcModules:    "/proc/modules",
		ProcMountInfo:  "/proc/self/mountinfo",
		SysModuleRoot:  "/sys/module",
		SysRoot:        "/sys",
		ModulesLoadDir: "/etc/modules-load.d",
		ModprobeDir:    "/etc/modprobe.d",
		Runner:         runner,
		HasModprobe: func() bool {
			_, err := exec.LookPath("modprobe")
			return err == nil
		},
	}
}

func (a *Applicator) Name() string        { return "kernelModule:" + a.Resource.Name }
func (a *Applicator) Description() string { return "kernel module " + a.Resource.Module }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("kernel module " + a.Resource.Module)
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	if a.HasModprobe == nil || !a.HasModprobe() {
		return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "kernel_module_provider_unsupported", DesiredSummary: desired, ObservedSummary: "modprobe is unavailable"}
	}
	loaded, err := a.loaded()
	if err != nil {
		return failed(desired, err)
	}
	if a.Resource.Loaded != nil && loaded != *a.Resource.Loaded {
		return drift(desired, "loaded state differs")
	}
	if a.Resource.Blacklisted != nil && *a.Resource.Blacklisted && loaded {
		return drift(desired, "blacklisted module remains loaded")
	}
	if a.Resource.Parameters != nil && a.Resource.Loaded != nil && *a.Resource.Loaded && loaded {
		for _, parameter := range a.Resource.ParameterNames() {
			value, err := os.ReadFile(filepath.Join(a.SysModuleRoot, normalized(a.Resource.Module), "parameters", parameter)) // #nosec G304 -- path derives from validated module and parameter names.
			if os.IsNotExist(err) {
				return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "kernel_module_parameter_unsupported", DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("module parameter " + parameter + " is unavailable")}
			}
			if err != nil {
				return failed(desired, err)
			}
			if strings.TrimSpace(string(value)) != a.Resource.Parameters[parameter] {
				return drift(desired, "module parameter differs")
			}
		}
	}
	if a.Resource.Persistent != nil {
		want := ""
		if *a.Resource.Persistent {
			want = a.Resource.Module + "\n"
		}
		if result := a.checkOwnedFile(a.modulesLoadPath(), want, desired, "boot declaration differs"); result != nil {
			return *result
		}
	}
	if a.Resource.Parameters != nil || a.Resource.Blacklisted != nil {
		if result := a.checkOwnedFile(a.modprobePath(), a.modprobeContent(), desired, "module options or blacklist differs"); result != nil {
			return *result
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) checkOwnedFile(path, want string, desired executor.RedactedSummary, message string) *executor.CheckResult {
	value, err := os.ReadFile(path) // #nosec G304 -- provider constructs a named owned fragment.
	if want == "" && os.IsNotExist(err) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		result := failed(desired, err)
		return &result
	}
	if string(value) != want {
		result := drift(desired, message)
		return &result
	}
	return nil
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.Unsupported || check.Status == executor.CheckFailed {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("cannot apply kernel module %s: %s", a.Resource.Module, check.ReasonCode)
	}

	loaded, err := a.loaded()
	if err != nil {
		return err
	}
	if a.requiresUnload(loaded) {
		if reason := a.protectionReason(); reason != "" {
			return fmt.Errorf("kernel module %s preflight blocked: %s", a.Resource.Module, reason)
		}
	}
	snapshots, err := a.captureOwnedFiles()
	if err != nil {
		return err
	}
	if err := a.applyOwnedFiles(); err != nil {
		return err
	}
	if err := a.applyRuntimeState(loaded); err != nil {
		if restoreErr := restoreOwnedFiles(snapshots); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore kernel module fragments: %w", restoreErr))
		}
		return err
	}
	return nil
}

func (a *Applicator) applyRuntimeState(loaded bool) error {
	if a.Resource.Blacklisted != nil && *a.Resource.Blacklisted {
		if loaded {
			return a.unload()
		}
		return nil
	}
	if a.Resource.Loaded == nil {
		return nil
	}
	if !*a.Resource.Loaded {
		if loaded {
			return a.unload()
		}
		return nil
	}
	if loaded && a.parametersDrift() {
		if err := a.unload(); err != nil {
			return err
		}
		loaded = false
	}
	if !loaded {
		args := []string{a.Resource.Module}
		for _, parameter := range a.Resource.ParameterNames() {
			args = append(args, parameter+"="+a.Resource.Parameters[parameter])
		}
		_, stderr, err := a.Runner.Run("modprobe", args...)
		if err != nil {
			return fmt.Errorf("load kernel module %s: %s: %w", a.Resource.Module, strings.TrimSpace(string(stderr)), err)
		}
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
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	if a.Resource.Loaded == nil && a.Resource.Persistent != nil {
		result.Activation = []executor.ActivationSignal{{Kind: executor.ActivationNextBoot}}
	}
	return result
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) applyOwnedFiles() error {
	if a.Resource.Persistent != nil {
		if *a.Resource.Persistent {
			if err := atomicWrite(a.modulesLoadPath(), []byte(a.Resource.Module+"\n"), 0o644); err != nil {
				return err
			}
		} else if err := removeOwnedFile(a.modulesLoadPath()); err != nil {
			return err
		}
	}
	if a.Resource.Parameters != nil || a.Resource.Blacklisted != nil {
		content := a.modprobeContent()
		if content == "" {
			return removeOwnedFile(a.modprobePath())
		}
		return atomicWrite(a.modprobePath(), []byte(content), 0o644)
	}
	return nil
}

type ownedFileSnapshot struct {
	path     string
	existed  bool
	content  []byte
	mode     fs.FileMode
	uid      int
	gid      int
	hasOwner bool
}

func (a *Applicator) captureOwnedFiles() ([]ownedFileSnapshot, error) {
	var paths []string
	if a.Resource.Persistent != nil {
		paths = append(paths, a.modulesLoadPath())
	}
	if a.Resource.Parameters != nil || a.Resource.Blacklisted != nil {
		paths = append(paths, a.modprobePath())
	}
	snapshots := make([]ownedFileSnapshot, 0, len(paths))
	for _, path := range paths {
		snapshot, err := captureOwnedFile(path)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func captureOwnedFile(path string) (ownedFileSnapshot, error) {
	snapshot := ownedFileSnapshot{path: path}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	if !info.Mode().IsRegular() {
		return snapshot, fmt.Errorf("kernel module fragment %s is not a regular file", path)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- provider constructs a named owned fragment.
	if err != nil {
		return snapshot, err
	}
	snapshot.existed = true
	snapshot.content = content
	snapshot.mode = info.Mode().Perm()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		snapshot.uid, snapshot.gid, snapshot.hasOwner = int(stat.Uid), int(stat.Gid), true
	}
	return snapshot, nil
}

func restoreOwnedFiles(snapshots []ownedFileSnapshot) error {
	var failures []error
	for _, snapshot := range snapshots {
		if !snapshot.existed {
			if err := removeOwnedFile(snapshot.path); err != nil {
				failures = append(failures, err)
			}
			continue
		}
		if err := atomicWrite(snapshot.path, snapshot.content, snapshot.mode); err != nil {
			failures = append(failures, err)
			continue
		}
		if snapshot.hasOwner {
			info, err := os.Stat(snapshot.path)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && (int(stat.Uid) != snapshot.uid || int(stat.Gid) != snapshot.gid) {
				if err := os.Chown(snapshot.path, snapshot.uid, snapshot.gid); err != nil {
					failures = append(failures, err)
				}
			}
		}
	}
	return errors.Join(failures...)
}

func (a *Applicator) loaded() (bool, error) {
	contents, err := os.ReadFile(a.ProcModules) // #nosec G304 -- provider path is injectable only for the OS boundary.
	if err != nil {
		return false, err
	}
	want := normalized(a.Resource.Module)
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && normalized(fields[0]) == want {
			return true, nil
		}
	}
	return false, nil
}

func (a *Applicator) parametersDrift() bool {
	if a.Resource.Parameters == nil {
		return false
	}
	for _, parameter := range a.Resource.ParameterNames() {
		value, err := os.ReadFile(filepath.Join(a.SysModuleRoot, normalized(a.Resource.Module), "parameters", parameter)) // #nosec G304 -- path derives from validated values.
		if err != nil || strings.TrimSpace(string(value)) != a.Resource.Parameters[parameter] {
			return true
		}
	}
	return false
}

func (a *Applicator) requiresUnload(loaded bool) bool {
	if !loaded {
		return false
	}
	if a.Resource.Blacklisted != nil && *a.Resource.Blacklisted {
		return true
	}
	if a.Resource.Loaded != nil && !*a.Resource.Loaded {
		return true
	}
	return a.Resource.Loaded != nil && *a.Resource.Loaded && a.parametersDrift()
}

func (a *Applicator) unload() error {
	_, stderr, err := a.Runner.Run("modprobe", "-r", a.Resource.Module)
	if err != nil {
		return fmt.Errorf("unload kernel module %s: %s: %w", a.Resource.Module, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (a *Applicator) protectionReason() string {
	want := normalized(a.Resource.Module)
	for _, module := range a.Resource.ProtectedModules {
		if normalized(module) == want {
			return "module is declared protected"
		}
	}
	if a.moduleBacksActiveRoot(want) {
		return "module backs the active root filesystem"
	}
	if a.moduleBacksActiveNetwork(want) {
		return "module backs the active network control path"
	}
	return ""
}

func (a *Applicator) moduleBacksActiveRoot(module string) bool {
	if a.moduleIsRootFilesystem(module) {
		return true
	}
	info, err := os.Stat("/")
	if err != nil {
		return true
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	major, minor := deviceNumbers(uint64(stat.Dev))
	return a.moduleAt(filepath.Join(a.SysRoot, "dev", "block", fmt.Sprintf("%d:%d", major, minor), "device", "driver", "module"), module)
}

func (a *Applicator) moduleIsRootFilesystem(module string) bool {
	contents, err := os.ReadFile(a.ProcMountInfo) // #nosec G304 -- the proc boundary is injectable for provider tests.
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(contents), "\n") {
		before, after, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		fields := strings.Fields(before)
		filesystem := strings.Fields(after)
		if len(fields) < 5 || fields[4] != "/" || len(filesystem) == 0 {
			continue
		}
		return normalized(filesystem[0]) == module
	}
	return false
}

func (a *Applicator) moduleBacksActiveNetwork(module string) bool {
	paths, err := filepath.Glob(filepath.Join(a.SysRoot, "class", "net", "*", "device", "driver", "module"))
	if err != nil {
		return true
	}
	for _, path := range paths {
		if a.moduleAt(path, module) {
			return true
		}
	}
	return false
}

func (a *Applicator) moduleAt(path, module string) bool {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return normalized(filepath.Base(target)) == module
}

func (a *Applicator) modulesLoadPath() string {
	return filepath.Join(a.ModulesLoadDir, "99-remotr-"+a.Resource.Name+".conf")
}

func (a *Applicator) modprobePath() string {
	return filepath.Join(a.ModprobeDir, "99-remotr-"+a.Resource.Name+".conf")
}

func (a *Applicator) modprobeContent() string {
	var lines []string
	if a.Resource.Parameters != nil {
		arguments := make([]string, 0, len(a.Resource.Parameters))
		for _, parameter := range a.Resource.ParameterNames() {
			arguments = append(arguments, parameter+"="+a.Resource.Parameters[parameter])
		}
		if len(arguments) > 0 {
			lines = append(lines, "options "+a.Resource.Module+" "+strings.Join(arguments, " "))
		}
	}
	if a.Resource.Blacklisted != nil && *a.Resource.Blacklisted {
		lines = append(lines, "blacklist "+a.Resource.Module)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func atomicWrite(path string, body []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-module-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func removeOwnedFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func normalized(module string) string { return strings.ReplaceAll(module, "-", "_") }

func deviceNumbers(dev uint64) (uint64, uint64) {
	return (dev>>8)&0xfff | (dev>>32)&0xfffff000, dev&0xff | (dev>>12)&0xffffff00
}

func drift(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{
		Status:          executor.Drifted,
		ReasonCode:      executor.ReasonStateDrift,
		DesiredSummary:  desired,
		ObservedSummary: executor.RedactedSummary(observed),
	}
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{
		Status:         executor.CheckFailed,
		ReasonCode:     executor.ReasonProbeFailed,
		DesiredSummary: desired,
		Err:            err,
	}
}
