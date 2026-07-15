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
)

const usersInteractive = "interactive"

// Applicator applies a file resource under each interactive user's home directory.
type Applicator struct {
	Resource  models.UserFileResource
	ListUsers func() ([]interactiveuser.Account, error)
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
	for _, u := range users {
		h, err := a.handlerFor(u)
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		_, met := h.State(ctx)
		if !met {
			return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("user " + u.Username + " differs")}
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "selected user files match"}
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
	anyApplied := false
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
	}
	if !anyApplied {
		return appErr.ErrStateAlreadyMet
	}
	return nil
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
