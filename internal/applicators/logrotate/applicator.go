// Package logrotate manages validated, named logrotate fragments.
package logrotate

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
	Resource          models.LogrotateResource
	FragmentsDir      string
	MainConfig        string
	Runner            executil.Runner
	ValidateEffective func(context.Context, string, string, string) error
	previous          []byte
	previousExists    bool
	rollbackArmed     bool
}

func New(resource models.LogrotateResource, runners ...executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	runner := executil.Runner(executil.SanitizedOSRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &Applicator{Resource: resource, FragmentsDir: "/etc/logrotate.d", MainConfig: "/etc/logrotate.conf", Runner: runner}
}

func (a *Applicator) Name() string { return "logrotate:" + a.Resource.Name }

func (a *Applicator) Description() string { return "logrotate fragment " + a.Resource.Name }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.FragmentsDir) || !filepath.IsAbs(a.MainConfig) {
		return "", fmt.Errorf("logrotate configuration paths must be absolute")
	}
	return filepath.Join(a.FragmentsDir, "remotr-"+a.Resource.Name), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named logrotate fragment " + a.Resource.Name)
	path, err := a.path()
	if err != nil {
		return failedCheck(desired, err)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated named fragment path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named logrotate fragment is absent"}
		}
		if err != nil {
			return failedCheck(desired, err)
		}
		return driftedCheck(desired, "named logrotate fragment exists")
	}
	if os.IsNotExist(err) {
		return driftedCheck(desired, "named logrotate fragment is absent")
	}
	if err != nil {
		return failedCheck(desired, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return failedCheck(desired, err)
	}
	if string(content) == a.render() && info.Mode().Perm() == 0o644 {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named logrotate fragment matches"}
	}
	return driftedCheck(desired, "named logrotate fragment differs")
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named fragment path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := a.validateCandidate(ctx); err != nil {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if err := atomicWrite(path, []byte(a.render())); err != nil {
		return err
	}
	a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), previous...), previousExists, true
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	}
}

func (a *Applicator) Revert(context.Context) error {
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if !a.previousExists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := atomicWrite(path, a.previous); err != nil {
		return err
	}
	a.previous = nil
	a.rollbackArmed = false
	return nil
}

func (a *Applicator) validateCandidate(ctx context.Context) error {
	stageRoot, err := os.MkdirTemp("", "remotr-logrotate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stagedDir := filepath.Join(stageRoot, "logrotate.d")
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		return err
	}
	if err := copyFragments(a.FragmentsDir, stagedDir); err != nil {
		return err
	}
	main, err := os.ReadFile(a.MainConfig) // #nosec G304 -- configured fixed main config path.
	if err != nil {
		return err
	}
	main, err = rewriteInclude(main, a.FragmentsDir, stagedDir)
	if err != nil {
		return err
	}
	stagedMain := filepath.Join(stageRoot, "logrotate.conf")
	if err := os.WriteFile(stagedMain, main, 0o644); err != nil {
		return err
	}
	stagedPath := filepath.Join(stagedDir, "remotr-"+a.Resource.Name)
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := os.WriteFile(stagedPath, []byte(a.render()), 0o644); err != nil {
		return err
	}
	if a.ValidateEffective != nil {
		return a.ValidateEffective(ctx, stagedMain, stagedDir, stagedPath)
	}
	_, stderr, err := a.Runner.Run("logrotate", "--debug", stagedMain)
	if err != nil {
		return fmt.Errorf("logrotate rejected staged effective configuration: %s", bounded(stderr))
	}
	return nil
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	var out strings.Builder
	out.WriteString(strings.Join(a.Resource.Paths, " "))
	out.WriteString(" {\n")
	fmt.Fprintf(&out, "  %s\n", a.Resource.Cadence)
	fmt.Fprintf(&out, "  rotate %d\n", *a.Resource.Retention)
	if a.Resource.Compress != nil {
		if *a.Resource.Compress {
			out.WriteString("  compress\n")
		} else {
			out.WriteString("  nocompress\n")
		}
	}
	if a.Resource.Create != nil {
		fmt.Fprintf(&out, "  create %s %s %s\n", a.Resource.Create.Mode, a.Resource.Create.Owner, a.Resource.Create.Group)
	}
	if a.Resource.Shared != nil {
		if *a.Resource.Shared {
			out.WriteString("  sharedscripts\n")
		} else {
			out.WriteString("  nosharedscripts\n")
		}
	}
	for _, script := range []struct {
		name  string
		value *models.LogrotateScript
	}{
		{"firstaction", a.Resource.FirstAction},
		{"prerotate", a.Resource.PreRotate},
		{"postrotate", a.Resource.PostRotate},
		{"lastaction", a.Resource.LastAction},
	} {
		if script.value == nil {
			continue
		}
		fmt.Fprintf(&out, "  %s\n    %s\n  endscript\n", script.name, shellCommand(script.value.Command))
	}
	out.WriteString("}\n")
	return out.String()
}

func shellCommand(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, argument := range argv {
		quoted = append(quoted, "'"+strings.ReplaceAll(argument, "'", `'"'"'`)+"'")
	}
	return strings.Join(quoted, " ")
}

func rewriteInclude(main []byte, activeDir, stagedDir string) ([]byte, error) {
	lines := strings.Split(string(main), "\n")
	found := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "include "+activeDir {
			lines[i] = "include " + stagedDir
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("logrotate main config does not include managed fragment directory %q", activeDir)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func copyFragments(source, destination string) error {
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("logrotate fragment %q is not regular", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(source, entry.Name())) // #nosec G304 -- enumerated fragment.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-logrotate-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func bounded(stderr []byte) string {
	const limit = 512
	value := strings.TrimSpace(string(stderr))
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

func failedCheck(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func driftedCheck(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}
