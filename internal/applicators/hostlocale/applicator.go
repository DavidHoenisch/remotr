// Package hostlocale applies independent timezone, locale, and console-keymap
// state through systemd's supported host-localization tools.
package hostlocale

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.HostLocaleResource
	Runner   executil.Runner
}

func New(resource models.HostLocaleResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner}
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
	if a.Resource.Timezone != nil {
		value, err := a.value("timedatectl", "show", "--property=Timezone", "--value")
		if err != nil {
			return failed(desired, err)
		}
		if value != *a.Resource.Timezone {
			return drifted(desired, "timezone differs")
		}
	}
	if a.Resource.Locale != nil || a.Resource.Keymap != nil {
		observed, keymap, err := a.localectlStatus()
		if err != nil {
			return failed(desired, err)
		}
		if a.Resource.Locale != nil {
			for key, want := range a.Resource.Locale {
				if observed[key] != want {
					return drifted(desired, "system locale differs")
				}
			}
		}
		if a.Resource.Keymap != nil && keymap != *a.Resource.Keymap {
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
	changed := changeSet{}
	if a.Resource.Timezone != nil {
		value, err := a.value("timedatectl", "show", "--property=Timezone", "--value")
		if err != nil {
			return changeSet{}, err
		}
		if value != *a.Resource.Timezone {
			if err := a.run("timedatectl", "set-timezone", *a.Resource.Timezone); err != nil {
				return changeSet{}, err
			}
			changed.timezone = true
		}
	}
	if a.Resource.Locale != nil || a.Resource.Keymap != nil {
		observed, keymap, err := a.localectlStatus()
		if err != nil {
			return changeSet{}, err
		}
		if a.Resource.Locale != nil {
			args := []string{"set-locale"}
			for _, key := range sortedKeys(a.Resource.Locale) {
				if observed[key] != a.Resource.Locale[key] {
					args = append(args, key+"="+a.Resource.Locale[key])
				}
			}
			if len(args) > 1 {
				if err := a.run("localectl", args...); err != nil {
					return changeSet{}, err
				}
				changed.locale = true
			}
		}
		if a.Resource.Keymap != nil && keymap != *a.Resource.Keymap {
			if err := a.run("localectl", "set-keymap", *a.Resource.Keymap); err != nil {
				return changeSet{}, err
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
