// Package desktopsettings manages typed dconf and GSettings policy without
// requiring an already active desktop login.
package desktopsettings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/fsops"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/selectorstate"
	"golang.org/x/sys/unix"
)

type Applicator struct {
	Resource    models.DesktopSettingResource
	Runner      executil.Runner
	ListUsers   func() ([]interactiveuser.Account, error)
	ConfigDir   string
	ProfilePath string
	StateDir    string
	StateKey    string
}

func New(resource models.DesktopSettingResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	if resource.Level == "" {
		resource.Level = models.DesktopSettingLevelDefault
	}
	return &Applicator{Resource: resource, Runner: runner, ConfigDir: "/etc/dconf/db/local.d", ProfilePath: "/etc/dconf/profile/user"}
}

func (a *Applicator) Name() string { return string(a.Resource.Provider) + ":" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("%s desktop setting %s", a.Resource.Provider, a.Resource.Name)
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("typed " + string(a.Resource.Value.Type) + " desktop setting")
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	if a.Resource.Scope == models.DesktopSettingScopeSystem {
		return a.checkSystem(desired)
	}
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return failed(desired, err)
	}
	if len(unresolved) > 0 {
		err := fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "unresolved_user_target", DesiredSummary: desired, ObservedSummary: "one or more user targets are unresolved", Err: err}
	}
	result := executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "selected desktop settings match"}
	for _, user := range users {
		observed, _, err := a.readUser(user)
		subresult := executor.CheckSubresult{Target: user.Username, Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		switch {
		case err != nil:
			subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.CheckFailed, executor.ReasonProbeFailed, "desktop setting probe failed"
			result.Status, result.ReasonCode, result.Err = executor.CheckFailed, executor.ReasonProbeFailed, fmt.Errorf("user %s desktop setting probe failed: %w", user.Username, err)
			result.ObservedSummary = "one or more desktop setting probes failed"
		case nativeEqual(a.Resource.Value, observed):
			subresult.ObservedSummary = "native typed value matches"
		default:
			subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "native typed value differs"
			if result.Status == executor.Compliant {
				result.Status, result.ReasonCode, result.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "one or more native typed values differ"
			}
		}
		appendSubresult(&result, subresult)
	}
	departures, err := a.authoritativeDepartures(users)
	if err != nil {
		return failed(desired, err)
	}
	for _, user := range departures {
		subresult := executor.CheckSubresult{Target: user.Username, Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "provider-owned desktop setting absent", ObservedSummary: "provider-owned desktop setting remains after selector departure"}
		appendSubresult(&result, subresult)
		if result.Status == executor.Compliant {
			result.Status, result.ReasonCode, result.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "provider-owned settings remain for users outside the authoritative selector"
		}
	}
	return result
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.Resource.Scope == models.DesktopSettingScopeSystem {
		check := a.Check(ctx)
		if check.Status == executor.Compliant {
			return appErr.ErrStateAlreadyMet
		}
		if check.Status != executor.Drifted {
			if check.Err != nil {
				return check.Err
			}
			return fmt.Errorf("desktop setting is not eligible for apply: %s", check.Status)
		}
		return a.applySystem()
	}
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return err
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
	}
	value, err := renderNative(a.Resource.Value)
	if err != nil {
		return err
	}
	store := a.ownershipStore()
	owners, err := store.Load()
	if err != nil {
		return err
	}
	ownersChanged := false
	anyApplied := false
	var failures []error
	for _, user := range users {
		observed, _, err := a.readUser(user)
		if err != nil {
			failures = append(failures, fmt.Errorf("user %s desktop setting probe failed: %w", user.Username, err))
			continue
		}
		if nativeEqual(a.Resource.Value, observed) {
			continue
		}
		var command string
		var args []string
		switch a.Resource.Provider {
		case models.DesktopSettingProviderDconf:
			command, args = "dconf", []string{"write", a.Resource.Path, value}
		case models.DesktopSettingProviderGSettings:
			command, args = "gsettings", []string{"set", a.Resource.Schema, a.Resource.Key, value}
		}
		if _, stderr, err := a.runAsUser(user, command, args...); err != nil {
			failures = append(failures, fmt.Errorf("user %s desktop setting apply failed: %s: %w", user.Username, bounded(stderr), err))
			continue
		}
		owners[user.Username] = struct{}{}
		ownersChanged = true
		anyApplied = true
	}
	departures, err := a.departures(users, owners)
	if err != nil {
		failures = append(failures, err)
	} else {
		for _, user := range departures {
			command, args := a.resetCommand()
			if _, stderr, err := a.runAsUser(user, command, args...); err != nil {
				failures = append(failures, fmt.Errorf("user %s desktop setting cleanup failed: %s: %w", user.Username, bounded(stderr), err))
				continue
			}
			delete(owners, user.Username)
			ownersChanged = true
			anyApplied = true
		}
	}
	if ownersChanged {
		if err := store.Save(owners); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if !anyApplied {
		return appErr.ErrStateAlreadyMet
	}
	return nil
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

func (a *Applicator) ownershipStore() selectorstate.Store {
	key := a.StateKey
	if key == "" {
		key = "desktopSetting/" + a.Resource.Name
	}
	return selectorstate.Store{StateDir: a.StateDir, Key: key}
}

func (a *Applicator) authoritativeDepartures(selected []interactiveuser.Account) ([]interactiveuser.Account, error) {
	if a.Resource.EffectiveSelectorOwnership() != models.OwnershipAuthoritative {
		return nil, nil
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return nil, fmt.Errorf("authoritative selector cleanup requires a state directory")
	}
	owners, err := a.ownershipStore().Load()
	if err != nil {
		return nil, err
	}
	return a.departures(selected, owners)
}

func (a *Applicator) departures(selected []interactiveuser.Account, owners map[string]struct{}) ([]interactiveuser.Account, error) {
	if a.Resource.EffectiveSelectorOwnership() != models.OwnershipAuthoritative {
		return nil, nil
	}
	selectedNames := make(map[string]struct{}, len(selected))
	for _, user := range selected {
		selectedNames[user.Username] = struct{}{}
	}
	all, err := a.listUsers()
	if err != nil {
		return nil, err
	}
	departures := make([]interactiveuser.Account, 0)
	for _, user := range all {
		_, selectedNow := selectedNames[user.Username]
		_, providerOwned := owners[user.Username]
		if providerOwned && !selectedNow {
			departures = append(departures, user)
		}
	}
	return departures, nil
}

func (a *Applicator) listUsers() ([]interactiveuser.Account, error) {
	list := a.ListUsers
	if list == nil {
		list = interactiveuser.List
	}
	return list()
}

func (a *Applicator) resetCommand() (string, []string) {
	if a.Resource.Provider == models.DesktopSettingProviderDconf {
		return "dconf", []string{"reset", a.Resource.Path}
	}
	return "gsettings", []string{"reset", a.Resource.Schema, a.Resource.Key}
}

func (a *Applicator) readUser(user interactiveuser.Account) (string, []byte, error) {
	switch a.Resource.Provider {
	case models.DesktopSettingProviderDconf:
		stdout, stderr, err := a.runAsUser(user, "dconf", "read", a.Resource.Path)
		return strings.TrimSpace(string(stdout)), stderr, err
	case models.DesktopSettingProviderGSettings:
		stdout, stderr, err := a.runAsUser(user, "gsettings", "get", a.Resource.Schema, a.Resource.Key)
		return strings.TrimSpace(string(stdout)), stderr, err
	default:
		return "", nil, fmt.Errorf("unsupported desktop setting provider %q", a.Resource.Provider)
	}
}

func (a *Applicator) runAsUser(user interactiveuser.Account, command string, args ...string) ([]byte, []byte, error) {
	if err := validateUserStatePath(user); err != nil {
		return nil, nil, err
	}
	commandArgs := []string{"-u", user.Username, "--", "env", "HOME=" + user.HomeDir, "dbus-run-session", "--", command}
	commandArgs = append(commandArgs, args...)
	return a.Runner.Run("runuser", commandArgs...)
}

func validateUserStatePath(user interactiveuser.Account) error {
	home := filepath.Clean(strings.TrimSpace(user.HomeDir))
	if !filepath.IsAbs(home) || home == string(os.PathSeparator) || home != user.HomeDir {
		return fmt.Errorf("user %s home path is invalid", user.Username)
	}
	fd, _, err := fsops.OpenSafeParent(filepath.Join(home, ".remotr-desktop-setting-home"), false)
	if err != nil {
		return fmt.Errorf("user %s home path is not a safe directory: %w", user.Username, err)
	}
	_ = unix.Close(fd)

	fd, _, err = fsops.OpenSafeParent(filepath.Join(home, ".config", "dconf", "user"), false)
	if err == nil {
		_ = unix.Close(fd)
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("user %s desktop state path is unsafe: %w", user.Username, err)
}

func (a *Applicator) checkSystem(desired executor.RedactedSummary) executor.CheckResult {
	configPath, lockPath := a.systemPaths()
	config, err := os.ReadFile(configPath) // #nosec G304 -- validated resource name under fixed dconf directory.
	if err != nil && !os.IsNotExist(err) {
		return failed(desired, err)
	}
	configMatches := err == nil && string(config) == a.renderSystem()
	lockMatches := true
	if a.Resource.Level == models.DesktopSettingLevelMandatory {
		lock, lockErr := os.ReadFile(lockPath) // #nosec G304 -- validated resource name under fixed dconf directory.
		if lockErr != nil && !os.IsNotExist(lockErr) {
			return failed(desired, lockErr)
		}
		lockMatches = lockErr == nil && string(lock) == a.Resource.Path+"\n"
	} else if _, lockErr := os.Lstat(lockPath); lockErr == nil {
		lockMatches = false
	} else if !os.IsNotExist(lockErr) {
		return failed(desired, lockErr)
	}
	profileMatches, err := a.systemProfileActive()
	if err != nil {
		return failed(desired, err)
	}
	if configMatches && lockMatches && profileMatches {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "persistent system dconf override matches"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "persistent system dconf override differs"}
}

func (a *Applicator) applySystem() error {
	configPath, lockPath := a.systemPaths()
	if err := atomicWrite(configPath, []byte(a.renderSystem())); err != nil {
		return err
	}
	if a.Resource.Level == models.DesktopSettingLevelMandatory {
		if err := atomicWrite(lockPath, []byte(a.Resource.Path+"\n")); err != nil {
			return err
		}
	} else if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := a.ensureSystemProfile(); err != nil {
		return err
	}
	_, stderr, err := a.Runner.Run("dconf", "update")
	if err != nil {
		return fmt.Errorf("dconf update failed: %s", bounded(stderr))
	}
	return nil
}

func (a *Applicator) systemProfileActive() (bool, error) {
	if strings.TrimSpace(a.ProfilePath) == "" {
		return true, nil
	}
	database, err := a.systemDatabase()
	if err != nil {
		return false, err
	}
	body, err := os.ReadFile(a.ProfilePath) // #nosec G304 -- configured root-owned dconf profile path.
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return profileContainsDatabase(body, database), nil
}

func (a *Applicator) ensureSystemProfile() error {
	if strings.TrimSpace(a.ProfilePath) == "" {
		return nil
	}
	database, err := a.systemDatabase()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(a.ProfilePath) // #nosec G304 -- configured root-owned dconf profile path.
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && profileContainsDatabase(body, database) {
		return nil
	}
	if os.IsNotExist(err) {
		body = []byte("user-db:user\n")
	} else if len(body) > 0 && body[len(body)-1] != '\n' {
		body = append(body, '\n')
	}
	body = append(body, []byte("system-db:"+database+"\n")...)
	return atomicWrite(a.ProfilePath, body)
}

func (a *Applicator) systemDatabase() (string, error) {
	base := filepath.Base(filepath.Clean(a.ConfigDir))
	database := strings.TrimSuffix(base, ".d")
	if database == "" || database == base {
		return "", fmt.Errorf("system dconf directory must name a database with a .d suffix")
	}
	for _, char := range database {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return "", fmt.Errorf("system dconf database name %q is invalid", database)
		}
	}
	return database, nil
}

func profileContainsDatabase(body []byte, database string) bool {
	wanted := "system-db:" + database
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == wanted {
			return true
		}
	}
	return false
}

func (a *Applicator) systemPaths() (string, string) {
	name := "90-remotr-" + a.Resource.Name
	return filepath.Join(a.ConfigDir, name), filepath.Join(a.ConfigDir, "locks", name)
}

func (a *Applicator) renderSystem() string {
	trimmed := strings.Trim(a.Resource.Path, "/")
	key := filepath.Base(trimmed)
	group := strings.TrimSuffix(trimmed, "/"+key)
	value, _ := renderNative(a.Resource.Value)
	return "[" + group + "]\n" + key + "=" + value + "\n"
}

func renderNative(value models.DesktopSettingValue) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	switch value.Type {
	case models.DesktopValueBoolean:
		return strconv.FormatBool(value.Value.(bool)), nil
	case models.DesktopValueString:
		return quoteGVariant(value.Value.(string)), nil
	case models.DesktopValueInt32:
		return strconv.FormatInt(signed(value.Value), 10), nil
	case models.DesktopValueInt64:
		return "int64 " + strconv.FormatInt(signed(value.Value), 10), nil
	case models.DesktopValueUint32:
		return "uint32 " + strconv.FormatInt(signed(value.Value), 10), nil
	case models.DesktopValueDouble:
		rendered := strconv.FormatFloat(value.Value.(float64), 'g', -1, 64)
		if !strings.ContainsAny(rendered, ".eE") {
			rendered += ".0"
		}
		return rendered, nil
	case models.DesktopValueStringList:
		values := stringValues(value.Value)
		if len(values) == 0 {
			return "@as []", nil
		}
		quoted := make([]string, len(values))
		for i, item := range values {
			quoted[i] = quoteGVariant(item)
		}
		return "[" + strings.Join(quoted, ", ") + "]", nil
	default:
		return "", fmt.Errorf("unsupported desktop value type %q", value.Type)
	}
}

func nativeEqual(desired models.DesktopSettingValue, observed string) bool {
	expected, err := renderNative(desired)
	if err != nil {
		return false
	}
	observed = strings.TrimSpace(observed)
	if desired.Type == models.DesktopValueString {
		value, ok := unquoteGVariant(observed)
		return ok && value == desired.Value.(string)
	}
	return observed == expected
}

func quoteGVariant(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), "'", `\'`) + "'"
}

func unquoteGVariant(value string) (string, bool) {
	if len(value) < 2 || (value[0] != '\'' && value[0] != '"') || value[len(value)-1] != value[0] {
		return "", false
	}
	return strings.ReplaceAll(strings.ReplaceAll(value[1:len(value)-1], `\'`, "'"), `\\`, `\`), true
}

func signed(value any) int64 {
	switch value := value.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case int32:
		return int64(value)
	default:
		return 0
	}
}

func stringValues(value any) []string {
	if values, ok := value.([]string); ok {
		return values
	}
	raw := value.([]any)
	values := make([]string, len(raw))
	for i := range raw {
		values[i] = raw[i].(string)
	}
	return values
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

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-desktop-setting-")
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
	const limit = 256
	value := strings.TrimSpace(string(stderr))
	if value == "" {
		return "provider returned no safe diagnostic"
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
