// Package sessionpolicy manages structured interactive desktop session state.
package sessionpolicy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/desktopsettings"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource  models.SessionPolicyResource
	Runner    executil.Runner
	ListUsers func() ([]interactiveuser.Account, error)
}

func New(resource models.SessionPolicyResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner}
}

func (a *Applicator) Name() string {
	return string(a.Resource.Provider) + "-session:" + a.Resource.Name
}

func (a *Applicator) Description() string { return "structured session policy " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("structured lock, idle, proxy, login, and application policy")
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	result := executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "selected session policy matches"}
	for _, binding := range a.bindings() {
		provider := desktopsettings.New(binding.resource(a.Resource), a.Runner)
		provider.ListUsers = a.ListUsers
		mergeCheck(&result, provider.Check(ctx))
	}
	if len(a.Resource.DefaultApplications) > 0 {
		mergeCheck(&result, a.checkDefaultApplications())
	}
	return result
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status != executor.Drifted {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("session policy is not eligible for apply: %s", check.Status)
	}
	for _, binding := range a.bindings() {
		provider := desktopsettings.New(binding.resource(a.Resource), a.Runner)
		provider.ListUsers = a.ListUsers
		if err := provider.Apply(ctx); err != nil && !errors.Is(err, appErr.ErrStateAlreadyMet) {
			return fmt.Errorf("session field %s: %w", binding.name, err)
		}
	}
	return a.applyDefaultApplications()
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

type settingBinding struct {
	name, path, schema, key string
	value                   models.DesktopSettingValue
}

func (b settingBinding) resource(policy models.SessionPolicyResource) models.DesktopSettingResource {
	return models.DesktopSettingResource{
		ResourceMeta: policy.ResourceMeta, Name: policy.Name + "-" + b.name, Provider: policy.Provider,
		Scope: models.DesktopSettingScopeUser, Selector: policy.Selector,
		Path: b.path, Schema: b.schema, Key: b.key, Value: b.value,
	}
}

func (a *Applicator) bindings() []settingBinding {
	var bindings []settingBinding
	add := func(name, dconfPath, schema, key string, value models.DesktopSettingValue) {
		bindings = append(bindings, settingBinding{name: name, path: dconfPath, schema: schema, key: key, value: value})
	}
	if a.Resource.LockEnabled != nil {
		add("lock-enabled", "/org/gnome/desktop/screensaver/lock-enabled", "org.gnome.desktop.screensaver", "lock-enabled", desktopValue(models.DesktopValueBoolean, *a.Resource.LockEnabled))
	}
	if a.Resource.IdleTimeoutSeconds != nil {
		add("idle-timeout", "/org/gnome/desktop/session/idle-delay", "org.gnome.desktop.session", "idle-delay", desktopValue(models.DesktopValueUint32, int64(*a.Resource.IdleTimeoutSeconds)))
	}
	if a.Resource.LockDelaySeconds != nil {
		add("lock-delay", "/org/gnome/desktop/screensaver/lock-delay", "org.gnome.desktop.screensaver", "lock-delay", desktopValue(models.DesktopValueUint32, int64(*a.Resource.LockDelaySeconds)))
	}
	if proxy := a.Resource.Proxy; proxy != nil {
		mode := string(proxy.Mode)
		if proxy.Mode == models.SessionProxyAutomatic {
			mode = "auto"
		}
		add("proxy-mode", "/org/gnome/system/proxy/mode", "org.gnome.system.proxy", "mode", desktopValue(models.DesktopValueString, mode))
		if proxy.AutomaticURL != "" {
			add("proxy-auto-url", "/org/gnome/system/proxy/autoconfig-url", "org.gnome.system.proxy", "autoconfig-url", desktopValue(models.DesktopValueString, proxy.AutomaticURL))
		}
		if len(proxy.IgnoreHosts) > 0 {
			add("proxy-ignore-hosts", "/org/gnome/system/proxy/ignore-hosts", "org.gnome.system.proxy", "ignore-hosts", desktopValue(models.DesktopValueStringList, append([]string(nil), proxy.IgnoreHosts...)))
		}
		if proxy.HTTPHost != "" {
			add("proxy-http-host", "/org/gnome/system/proxy/http/host", "org.gnome.system.proxy.http", "host", desktopValue(models.DesktopValueString, proxy.HTTPHost))
			add("proxy-http-port", "/org/gnome/system/proxy/http/port", "org.gnome.system.proxy.http", "port", desktopValue(models.DesktopValueInt32, int64(proxy.HTTPPort)))
		}
		if proxy.HTTPSHost != "" {
			add("proxy-https-host", "/org/gnome/system/proxy/https/host", "org.gnome.system.proxy.https", "host", desktopValue(models.DesktopValueString, proxy.HTTPSHost))
			add("proxy-https-port", "/org/gnome/system/proxy/https/port", "org.gnome.system.proxy.https", "port", desktopValue(models.DesktopValueInt32, int64(proxy.HTTPSPort)))
		}
	}
	for _, lockdown := range []struct {
		name, key string
		value     *bool
	}{
		{"disable-user-switching", "disable-user-switching", a.Resource.DisableUserSwitching},
		{"disable-logout", "disable-log-out", a.Resource.DisableLogout},
		{"disable-command-line", "disable-command-line", a.Resource.DisableCommandLine},
	} {
		if lockdown.value != nil {
			add(lockdown.name, "/org/gnome/desktop/lockdown/"+lockdown.key, "org.gnome.desktop.lockdown", lockdown.key, desktopValue(models.DesktopValueBoolean, *lockdown.value))
		}
	}
	if a.Resource.Provider == models.DesktopSettingProviderDconf {
		for i := range bindings {
			bindings[i].schema, bindings[i].key = "", ""
		}
	} else {
		for i := range bindings {
			bindings[i].path = ""
		}
	}
	return bindings
}

func desktopValue(valueType models.DesktopValueType, value any) models.DesktopSettingValue {
	return models.DesktopSettingValue{Type: valueType, Value: value}
}

func (a *Applicator) checkDefaultApplications() executor.CheckResult {
	desired := executor.RedactedSummary("structured default applications")
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return failed(desired, err)
	}
	if len(unresolved) > 0 {
		err := fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "unresolved_user_target", DesiredSummary: desired, Err: err}
	}
	result := executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "default applications match"}
	mimes := sortedMIMEs(a.Resource.DefaultApplications)
	for _, user := range users {
		subresult := executor.CheckSubresult{Target: user.Username, Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "default applications match"}
		for _, mime := range mimes {
			stdout, _, err := a.runAsUser(user, "xdg-mime", "query", "default", mime)
			if err != nil {
				subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.CheckFailed, executor.ReasonProbeFailed, "default application probe failed"
				result.Status, result.ReasonCode, result.Err = executor.CheckFailed, executor.ReasonProbeFailed, fmt.Errorf("user %s default application probe failed: %w", user.Username, err)
				break
			}
			if strings.TrimSpace(string(stdout)) != a.Resource.DefaultApplications[mime] {
				subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "default application differs"
				if result.Status == executor.Compliant {
					result.Status, result.ReasonCode, result.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "one or more default applications differ"
				}
			}
		}
		appendSubresult(&result, subresult)
	}
	return result
}

func (a *Applicator) applyDefaultApplications() error {
	if len(a.Resource.DefaultApplications) == 0 {
		return nil
	}
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return err
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
	}
	for _, user := range users {
		for _, mime := range sortedMIMEs(a.Resource.DefaultApplications) {
			stdout, _, err := a.runAsUser(user, "xdg-mime", "query", "default", mime)
			if err == nil && strings.TrimSpace(string(stdout)) == a.Resource.DefaultApplications[mime] {
				continue
			}
			if _, _, err := a.runAsUser(user, "xdg-mime", "default", a.Resource.DefaultApplications[mime], mime); err != nil {
				return fmt.Errorf("user %s default application apply failed", user.Username)
			}
		}
	}
	return nil
}

func (a *Applicator) selectedUsers() ([]interactiveuser.Account, []string, error) {
	list := a.ListUsers
	if list == nil {
		list = interactiveuser.List
	}
	users, err := list()
	if err != nil {
		return nil, nil, err
	}
	return interactiveuser.Select(users, a.Resource.Selector)
}

func (a *Applicator) runAsUser(user interactiveuser.Account, command string, args ...string) ([]byte, []byte, error) {
	commandArgs := []string{"-u", user.Username, "--", "env", "HOME=" + user.HomeDir, "dbus-run-session", "--", command}
	commandArgs = append(commandArgs, args...)
	return a.Runner.Run("runuser", commandArgs...)
}

func sortedMIMEs(applications map[string]string) []string {
	keys := make([]string, 0, len(applications))
	for key := range applications {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeCheck(result *executor.CheckResult, child executor.CheckResult) {
	if child.Status == executor.CheckFailed {
		result.Status, result.ReasonCode, result.Err = executor.CheckFailed, child.ReasonCode, child.Err
		result.ObservedSummary = "one or more session policy probes failed"
	} else if child.Status != executor.Compliant && result.Status == executor.Compliant {
		result.Status, result.ReasonCode = child.Status, child.ReasonCode
		result.ObservedSummary = "one or more session policy fields differ"
	}
	byTarget := make(map[string]int, len(result.Subresults))
	for i, subresult := range result.Subresults {
		byTarget[subresult.Target] = i
	}
	for _, subresult := range child.Subresults {
		if index, exists := byTarget[subresult.Target]; exists {
			if checkRank(subresult.Status) > checkRank(result.Subresults[index].Status) {
				result.Subresults[index] = subresult
			}
			continue
		}
		appendSubresult(result, subresult)
		byTarget[subresult.Target] = len(result.Subresults) - 1
	}
	result.SubresultsTruncated = result.SubresultsTruncated || child.SubresultsTruncated
}

func checkRank(status executor.CheckStatus) int {
	switch status {
	case executor.CheckFailed:
		return 4
	case executor.Unsupported:
		return 3
	case executor.Deferred:
		return 2
	case executor.Drifted:
		return 1
	default:
		return 0
	}
}

func appendSubresult(result *executor.CheckResult, subresult executor.CheckSubresult) {
	if len(result.Subresults) < executor.MaxCheckSubresults {
		result.Subresults = append(result.Subresults, subresult)
	} else {
		result.SubresultsTruncated = true
	}
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
