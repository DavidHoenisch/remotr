package users

import (
	"context"
	"errors"
	"os/user"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/userutil"
)

type Applicator struct {
	Resource models.UserResource
	AddFunc  func(uname string) error
	DelFunc  func(uname string) error
}

func New(r models.UserResource) *Applicator {
	return &Applicator{
		Resource: r,
		AddFunc:  userutil.Useradd,
		DelFunc:  userutil.Userdel,
	}
}

func (a *Applicator) Name() string { return "user:" + a.Resource.Name }

func (a *Applicator) Description() string {
	if a.Resource.Present {
		return "ensure user " + a.Resource.Username
	}
	return "remove user " + a.Resource.Username
}

func (a *Applicator) exists() bool {
	_, err := user.Lookup(a.Resource.Username)
	return err == nil
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	ex := a.exists()
	if a.Resource.Present {
		return ex, ex
	}
	return ex, !ex
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.Present {
		return a.AddFunc(a.Resource.Username)
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
