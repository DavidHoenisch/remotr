// Package hostlocale applies independent timezone, locale, and console-keymap
// state through systemd's supported host-localization tools.
package hostlocale

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource           models.HostLocaleResource
	Runner             executil.Runner
	KeyboardConfigPath string
}

var errKeymapCompilerUnsupported = errors.New("host locale keymap compiler unsupported")

func New(resource models.HostLocaleResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner, KeyboardConfigPath: "/etc/default/keyboard"}
}

func (a *Applicator) Name() string        { return "host-locale:" + a.Resource.Name }
func (a *Applicator) Description() string { return "host locale " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("managed host locale " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return failed(desired, err)
	}
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	if err := a.validateKeymap(); err != nil {
		if errors.Is(err, errKeymapCompilerUnsupported) {
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "host_locale_keymap_unsupported", DesiredSummary: desired, ObservedSummary: "the Ubuntu console keymap compiler is unavailable or rejected the requested layout"}
		}
		return failed(desired, err)
	}
	if a.Resource.Timezone != nil {
		value, err := a.value("timedatectl", "show", "--property=Timezone", "--value")
		if err != nil {
			return failed(desired, err)
		}
		if value != *a.Resource.Timezone {
			return drifted(desired, "timezone differs")
		}
	}
	if a.Resource.Locale != nil {
		observed, _, err := a.localectlStatus()
		if err != nil {
			return failed(desired, err)
		}
		for key, want := range a.Resource.Locale {
			if observed[key] != want {
				return drifted(desired, "system locale differs")
			}
		}
	}
	if a.Resource.Keymap != nil {
		keyboard, err := a.keyboardConfig()
		if err != nil {
			return failed(desired, err)
		}
		if keyboard.layout != *a.Resource.Keymap {
			return drifted(desired, "console keymap differs")
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) Apply(ctx context.Context) error {
	_, err := a.apply(ctx)
	return err
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	changed, err := a.apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	if changed.locale {
		result.Activation = append(result.Activation, executor.ActivationSignal{Kind: executor.ActivationLogoutRequired})
	}
	if changed.keymap {
		result.Activation = append(result.Activation, executor.ActivationSignal{Kind: executor.ActivationRebootRequired})
		result.RebootRequired = executor.RebootRequired
	}
	return result
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

type changeSet struct{ timezone, locale, keymap bool }

func (a *Applicator) apply(ctx context.Context) (changeSet, error) {
	if err := ctx.Err(); err != nil {
		return changeSet{}, err
	}
	if err := a.Resource.Validate(); err != nil {
		return changeSet{}, err
	}
	if err := a.validateKeymap(); err != nil {
		return changeSet{}, err
	}
	changed := changeSet{}
	originalTimezone := ""
	originalLocale := []string(nil)
	rollback := func(cause error) error {
		errList := []error{cause}
		if changed.locale && len(originalLocale) > 0 {
			if err := a.run("localectl", append([]string{"set-locale"}, originalLocale...)...); err != nil {
				errList = append(errList, fmt.Errorf("restore locale: %w", err))
			}
		}
		if changed.timezone {
			if err := a.run("timedatectl", "set-timezone", originalTimezone); err != nil {
				errList = append(errList, fmt.Errorf("restore timezone: %w", err))
			}
		}
		return errors.Join(errList...)
	}
	if a.Resource.Timezone != nil {
		value, err := a.value("timedatectl", "show", "--property=Timezone", "--value")
		if err != nil {
			return changeSet{}, err
		}
		if value != *a.Resource.Timezone {
			if err := a.run("timedatectl", "set-timezone", *a.Resource.Timezone); err != nil {
				return changeSet{}, err
			}
			originalTimezone = value
			changed.timezone = true
		}
	}
	if a.Resource.Locale != nil {
		observed, _, err := a.localectlStatus()
		if err != nil {
			return changeSet{}, rollback(err)
		}
		args := []string{"set-locale"}
		for _, key := range sortedKeys(a.Resource.Locale) {
			if observed[key] != a.Resource.Locale[key] {
				args = append(args, key+"="+a.Resource.Locale[key])
				originalLocale = append(originalLocale, key+"="+observed[key])
			}
		}
		if len(args) > 1 {
			if err := a.run("localectl", args...); err != nil {
				return changeSet{}, rollback(err)
			}
			changed.locale = true
		}
	}
	if a.Resource.Keymap != nil {
		keyboard, err := a.keyboardConfig()
		if err != nil {
			return changeSet{}, rollback(err)
		}
		if keyboard.layout != *a.Resource.Keymap {
			body := replaceKeyboardLayout(keyboard.body, *a.Resource.Keymap)
			if err := writeKeyboardConfig(a.keyboardConfigPath(), body, keyboard); err != nil {
				return changeSet{}, rollback(err)
			}
			changed.keymap = true
		}
	}
	if !changed.timezone && !changed.locale && !changed.keymap {
		return changeSet{}, appErr.ErrStateAlreadyMet
	}
	return changed, nil
}

func (a *Applicator) value(command string, args ...string) (string, error) {
	stdout, stderr, err := a.Runner.Run(command, args...)
	if err != nil {
		return "", fmt.Errorf("read host locale with %s: %s: %w", command, strings.TrimSpace(string(stderr)), err)
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (a *Applicator) run(command string, args ...string) error {
	if _, stderr, err := a.Runner.Run(command, args...); err != nil {
		return fmt.Errorf("apply host locale with %s: %s: %w", command, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (a *Applicator) localectlStatus() (map[string]string, string, error) {
	stdout, stderr, err := a.Runner.Run("localectl", "status", "--no-pager")
	if err != nil {
		return nil, "", fmt.Errorf("read host locale with localectl: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	locale, keymap := parseLocalectlStatus(string(stdout))
	return locale, keymap, nil
}

func (a *Applicator) validateKeymap() error {
	if a.Resource.Keymap == nil {
		return nil
	}
	_, stderr, err := a.Runner.Run("ckbcomp", *a.Resource.Keymap)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errKeymapCompilerUnsupported, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

type keyboardState struct {
	body   []byte
	layout string
	mode   os.FileMode
	uid    int
	gid    int
}

func (a *Applicator) keyboardConfigPath() string {
	if a.KeyboardConfigPath == "" {
		return "/etc/default/keyboard"
	}
	return a.KeyboardConfigPath
}

func (a *Applicator) keyboardConfig() (keyboardState, error) {
	path := a.keyboardConfigPath()
	info, err := os.Lstat(path)
	if err != nil {
		return keyboardState{}, fmt.Errorf("read host keymap configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return keyboardState{}, fmt.Errorf("host keymap configuration %q must be a regular file", path)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- fixed provider path or injected OS-boundary fixture.
	if err != nil {
		return keyboardState{}, fmt.Errorf("read host keymap configuration: %w", err)
	}
	layout, err := parseKeyboardLayout(body)
	if err != nil {
		return keyboardState{}, err
	}
	uid, gid := -1, -1
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid, gid = int(stat.Uid), int(stat.Gid)
	}
	return keyboardState{body: body, layout: layout, mode: info.Mode().Perm(), uid: uid, gid: gid}, nil
}

func parseKeyboardLayout(body []byte) (string, error) {
	layout := ""
	found := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "XKBLAYOUT" {
			continue
		}
		if found {
			return "", errors.New("host keymap configuration contains duplicate XKBLAYOUT assignments")
		}
		found = true
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		layout = value
	}
	return layout, nil
}

func replaceKeyboardLayout(body []byte, layout string) []byte {
	lines := strings.SplitAfter(string(body), "\n")
	replacement := "XKBLAYOUT=\"" + layout + "\""
	for index, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		key, _, ok := strings.Cut(trimmed, "=")
		if ok && strings.TrimSpace(key) == "XKBLAYOUT" {
			if strings.HasSuffix(line, "\n") {
				replacement += "\n"
			}
			lines[index] = replacement
			return []byte(strings.Join(lines, ""))
		}
	}
	if len(body) > 0 && body[len(body)-1] != '\n' {
		replacement = "\n" + replacement
	}
	return append(append([]byte(nil), body...), []byte(replacement+"\n")...)
}

func writeKeyboardConfig(path string, body []byte, previous keyboardState) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-keyboard-")
	if err != nil {
		return fmt.Errorf("stage host keymap configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(previous.mode); err != nil {
		return fmt.Errorf("set staged host keymap mode: %w", err)
	}
	if stat, ok := previousFileOwner(temporary); ok && (stat.uid != previous.uid || stat.gid != previous.gid) {
		if err := temporary.Chown(previous.uid, previous.gid); err != nil {
			return fmt.Errorf("set staged host keymap owner: %w", err)
		}
	}
	if _, err := temporary.Write(body); err != nil {
		return fmt.Errorf("write staged host keymap configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync staged host keymap configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close staged host keymap configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace host keymap configuration: %w", err)
	}
	cleanup = false
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open host keymap directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync host keymap directory: %w", err)
	}
	return nil
}

type fileOwner struct{ uid, gid int }

func previousFileOwner(file *os.File) (fileOwner, bool) {
	info, err := file.Stat()
	if err != nil {
		return fileOwner{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileOwner{}, false
	}
	return fileOwner{uid: int(stat.Uid), gid: int(stat.Gid)}, true
}

func parseLocalectlStatus(value string) (map[string]string, string) {
	parsed := make(map[string]string)
	keymap := ""
	readingLocale := false
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "System Locale:"); ok {
			readingLocale = true
			parseLocaleFields(parsed, after)
			continue
		}
		if after, ok := strings.CutPrefix(trimmed, "VC Keymap:"); ok {
			keymap = strings.TrimSpace(after)
			readingLocale = false
			continue
		}
		if readingLocale && strings.HasPrefix(line, " ") {
			parseLocaleFields(parsed, trimmed)
			continue
		}
		readingLocale = false
	}
	return parsed, keymap
}

func parseLocaleFields(parsed map[string]string, value string) {
	for _, field := range strings.Fields(value) {
		key, item, ok := strings.Cut(field, "=")
		if ok {
			parsed[key] = item
		}
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func drifted(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
