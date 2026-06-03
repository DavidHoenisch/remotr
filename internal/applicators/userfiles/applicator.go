package userfiles

import (
	"context"
	"fmt"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/applicators/files"
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

func (a *Applicator) handlerFor(u interactiveuser.Account) (*files.Applicator, error) {
	abs, err := interactiveuser.HomePath(u.HomeDir, a.Resource.Path)
	if err != nil {
		return nil, fmt.Errorf("user %s: %w", u.Username, err)
	}
	return files.NewOwned(a.Resource.ToFile(abs), u.UID, u.GID), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	users, err := a.listUsers()
	if err != nil {
		return nil, false
	}
	if len(users) == 0 {
		return nil, strings.TrimSpace(a.Resource.Content) == "" && a.Resource.WithRegx == ""
	}
	for _, u := range users {
		h, err := a.handlerFor(u)
		if err != nil {
			return nil, false
		}
		_, met := h.State(ctx)
		if !met {
			return nil, false
		}
	}
	return nil, true
}

func (a *Applicator) Apply(ctx context.Context) error {
	users, err := a.listUsers()
	if err != nil {
		return err
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
	users, err := a.listUsers()
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
