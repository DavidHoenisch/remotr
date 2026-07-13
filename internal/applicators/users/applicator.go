package users

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"strconv"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/userutil"
)

type Applicator struct {
	Resource      models.UserResource
	AddFunc       func(uname string) error
	DelFunc       func(uname string) error
	LookupFunc    func(string) (*user.User, error)
	AddUIDFunc    func(string, int) error
	ModifyUIDFunc func(string, int) error
}

func New(r models.UserResource) *Applicator {
	return &Applicator{
		Resource:      r,
		AddFunc:       userutil.Useradd,
		DelFunc:       userutil.Userdel,
		LookupFunc:    user.Lookup,
		AddUIDFunc:    userutil.UseraddUID,
		ModifyUIDFunc: userutil.UsermodUID,
	}
}

func (a *Applicator) Name() string { return "user:" + a.Resource.Name }

func (a *Applicator) Description() string {
	if a.Resource.Present {
		return "ensure user " + a.Resource.Username
	}
	return "remove user " + a.Resource.Username
}

func (a *Applicator) lookup() (*user.User, error) {
	return a.LookupFunc(a.Resource.Username)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	u, err := a.lookup()
	ex := err == nil
	if a.Resource.Present {
		if !ex {
			return nil, false
		}
		if a.Resource.UID > 0 {
			uid, parseErr := strconv.Atoi(u.Uid)
			if parseErr != nil || uid != a.Resource.UID {
				return u, false
			}
		}
		return u, true
	}
	return ex, !ex
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.Present {
		u, err := a.lookup()
		if err != nil {
			if a.Resource.UID > 0 {
				return a.AddUIDFunc(a.Resource.Username, a.Resource.UID)
			}
			return a.AddFunc(a.Resource.Username)
		}
		current, err := strconv.Atoi(u.Uid)
		if err != nil {
			return fmt.Errorf("user %q has invalid uid %q", a.Resource.Username, u.Uid)
		}
		if a.Resource.UID > 0 && current != a.Resource.UID {
			if !a.Resource.AllowUIDReassignment {
				return fmt.Errorf("user %q uid reassignment from %d to %d requires allowUIDReassignment", a.Resource.Username, current, a.Resource.UID)
			}
			return a.ModifyUIDFunc(a.Resource.Username, a.Resource.UID)
		}
		return appErr.ErrStateAlreadyMet
	}
	return a.DelFunc(a.Resource.Username)
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

// Exists reports whether the username is present.
func Exists(username string) bool {
	_, err := user.Lookup(username)
	if errors.As(err, new(user.UnknownUserError)) {
		return false
	}
	return err == nil
}
