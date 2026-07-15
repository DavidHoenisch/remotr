package userfiles

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/selectorstate"
)

const usersInteractive = "interactive"

// Applicator applies a file resource under each interactive user's home directory.
type Applicator struct {
	Resource  models.UserFileResource
	ListUsers func() ([]interactiveuser.Account, error)
	StateDir  string
	StateKey  string
}

func New(r models.UserFileResource) *Applicator {
	return &Applicator{Resource: r}
}

func (a *Applicator) Name() string { return "userFile:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("user file %s for %s users", a.Resource.Path, a.Resource.Users)
}

func (a *Applicator) listUsers() ([]interactiveuser.Account, error) {
	fn := a.ListUsers
	if fn == nil {
		fn = interactiveuser.List
	}
	return fn()
}

func (a *Applicator) selectedUsers() ([]interactiveuser.Account, []string, error) {
	users, err := a.listUsers()
	if err != nil {
		return nil, nil, err
	}
	return interactiveuser.Select(users, a.Resource.EffectiveSelector())
}

func (a *Applicator) handlerFor(u interactiveuser.Account) (*files.Applicator, error) {
	abs, err := interactiveuser.HomePath(u.HomeDir, a.Resource.Path)
	if err != nil {
		return nil, fmt.Errorf("user %s: %w", u.Username, err)
	}
	return files.NewOwnedUnder(a.Resource.ToFile(abs), u.HomeDir, u.UID, u.GID), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("user file for selected interactive users")
	if err := a.Resource.Validate(); err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	if len(unresolved) > 0 {
		err := fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
		return executor.CheckResult{
			Status: executor.CheckFailed, ReasonCode: executor.ReasonCode("unresolved_user_target"), DesiredSummary: desired,
			ObservedSummary: executor.RedactedSummary(err.Error()), Err: err,
		}
	}
	if len(users) == 0 {
		if strings.TrimSpace(a.Resource.Content) == "" && a.Resource.WithRegx == "" {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "no interactive users selected"}
	}
	result := executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "selected user files match"}
	for _, u := range users {
		subresult := executor.CheckSubresult{Target: u.Username, Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "owned user file matches"}
		h, err := a.handlerFor(u)
		if err != nil {
			subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.CheckFailed, executor.ReasonProbeFailed, "user path could not be inspected"
			result.Status, result.ReasonCode, result.Err = executor.CheckFailed, executor.ReasonProbeFailed, err
			appendSubresult(&result, subresult)
			continue
		}
		_, met := h.State(ctx)
		if !met {
			subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "owned user file differs"
			if result.Status == executor.Compliant {
				result.Status, result.ReasonCode, result.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "one or more selected user files differ"
			}
		} else {
			subresult.ObservedSummary = "owned user file matches"
		}
		appendSubresult(&result, subresult)
	}
	departures, _, err := a.authoritativeDepartures(users)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	for _, user := range departures {
		path, err := interactiveuser.HomePath(user.HomeDir, a.Resource.Path)
		subresult := executor.CheckSubresult{Target: user.Username, Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "provider-owned user file absent", ObservedSummary: "provider-owned user file remains after selector departure"}
		if err == nil {
			_, exists, readErr := files.ReadOwnedUnder(user.HomeDir, path)
			if readErr != nil {
				err = readErr
			} else if !exists {
				subresult.ObservedSummary = "stale provider ownership record remains after selector departure"
			}
		}
		if err != nil {
			subresult.Status, subresult.ReasonCode, subresult.ObservedSummary = executor.CheckFailed, executor.ReasonProbeFailed, "departed user path could not be inspected safely"
			result.Status, result.ReasonCode, result.Err = executor.CheckFailed, executor.ReasonProbeFailed, fmt.Errorf("user %s cleanup check: %w", user.Username, err)
		} else if result.Status == executor.Compliant {
			result.Status, result.ReasonCode, result.ObservedSummary = executor.Drifted, executor.ReasonStateDrift, "provider-owned user files remain outside the authoritative selector"
		}
		appendSubresult(&result, subresult)
	}
	return result
}

func appendSubresult(result *executor.CheckResult, subresult executor.CheckSubresult) {
	if len(result.Subresults) < executor.MaxCheckSubresults {
		result.Subresults = append(result.Subresults, subresult)
		return
	}
	result.SubresultsTruncated = true
}

func (a *Applicator) Apply(ctx context.Context) error {
	users, unresolved, err := a.selectedUsers()
	if err != nil {
		return err
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("unresolved interactive user targets: %s", strings.Join(unresolved, ", "))
	}
	if len(users) == 0 {
		return fmt.Errorf("no interactive users found")
	}
	store := a.ownershipStore()
	owners, err := store.Load()
	if err != nil {
		return err
	}
	anyApplied := false
	ownersChanged := false
	for _, u := range users {
		h, err := a.handlerFor(u)
		if err != nil {
			return err
		}
		_, met := h.State(ctx)
		if met {
			continue
		}
		if err := h.Apply(ctx); err != nil {
			if err == appErr.ErrStateAlreadyMet {
				continue
			}
			return fmt.Errorf("user %s: %w", u.Username, err)
		}
		anyApplied = true
		owners[u.Username] = struct{}{}
		ownersChanged = true
	}
	departures, _, err := a.departures(users, owners)
	if err != nil {
		return err
	}
	for _, user := range departures {
		path, err := interactiveuser.HomePath(user.HomeDir, a.Resource.Path)
		if err != nil {
			return fmt.Errorf("user %s cleanup path: %w", user.Username, err)
		}
		_, exists, err := files.ReadOwnedUnder(user.HomeDir, path)
		if err != nil {
			return fmt.Errorf("user %s cleanup check: %w", user.Username, err)
		}
		if exists {
			handler, err := a.cleanupHandlerFor(user)
			if err != nil {
				return err
			}
			if err := handler.Apply(ctx); err != nil && err != appErr.ErrStateAlreadyMet {
				return fmt.Errorf("user %s cleanup: %w", user.Username, err)
			}
			anyApplied = true
		}
		delete(owners, user.Username)
		ownersChanged = true
	}
	if ownersChanged {
		if err := store.Save(owners); err != nil {
			return err
		}
	}
	if !anyApplied {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}

func (a *Applicator) cleanupHandlerFor(user interactiveuser.Account) (*files.Applicator, error) {
	resource := a.Resource
	resource.Lifecycle = models.LifecycleAbsent
	abs, err := interactiveuser.HomePath(user.HomeDir, resource.Path)
	if err != nil {
		return nil, err
	}
	return files.NewOwnedUnder(resource.ToFile(abs), user.HomeDir, user.UID, user.GID), nil
}

func (a *Applicator) ownershipStore() selectorstate.Store {
	key := a.StateKey
	if key == "" {
		key = "userFile/" + a.Resource.Name
	}
	return selectorstate.Store{StateDir: a.StateDir, Key: key}
}

func (a *Applicator) authoritativeDepartures(selected []interactiveuser.Account) ([]interactiveuser.Account, map[string]struct{}, error) {
	if a.Resource.EffectiveSelectorOwnership() != models.OwnershipAuthoritative {
		return nil, nil, nil
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return nil, nil, fmt.Errorf("authoritative selector cleanup requires a state directory")
	}
	owners, err := a.ownershipStore().Load()
	if err != nil {
		return nil, nil, err
	}
	departures, _, err := a.departures(selected, owners)
	return departures, owners, err
}

func (a *Applicator) departures(selected []interactiveuser.Account, owners map[string]struct{}) ([]interactiveuser.Account, map[string]struct{}, error) {
	if a.Resource.EffectiveSelectorOwnership() != models.OwnershipAuthoritative {
		return nil, owners, nil
	}
	selectedNames := make(map[string]struct{}, len(selected))
	for _, user := range selected {
		selectedNames[user.Username] = struct{}{}
	}
	all, err := a.listUsers()
	if err != nil {
		return nil, owners, err
	}
	departures := make([]interactiveuser.Account, 0)
	for _, user := range all {
		_, selectedNow := selectedNames[user.Username]
		_, providerOwned := owners[user.Username]
		if providerOwned && !selectedNow {
			departures = append(departures, user)
		}
	}
	return departures, owners, nil
}

func (a *Applicator) Revert(ctx context.Context) error {
	users, _, err := a.selectedUsers()
	if err != nil {
		return err
	}
	var first error
	for _, u := range users {
		h, err := a.handlerFor(u)
		if err != nil {
			return err
		}
		if err := h.Revert(ctx); err != nil && first == nil {
			first = fmt.Errorf("user %s: %w", u.Username, err)
		}
	}
	return first
}
