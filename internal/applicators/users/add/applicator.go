package add

import (
	"context"
	"errors"
	"log"
	"os/user"

	Err "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/userutil"
)

type AddUserApplicator struct {
	Username string
	Uid      int
	AddFunc  func(uname string) error
}

func (r *AddUserApplicator) Name() string { return "AddUserApplicator" }

func (r *AddUserApplicator) Description() string { return "Add a user account to system" }

// returns true if the desired state is already met, false
// if not
func (r *AddUserApplicator) State(ctx context.Context) (any, bool) {
	u, err := user.Lookup(r.Username)

	if errors.As(err, new(user.UnknownUserError)) {
		return nil, false
	}

	log.Printf("User exists as %s", u.Uid)

	return u, true
}

func (r *AddUserApplicator) Apply(ctx context.Context) error {

	_, state := r.State(ctx)
	switch state {
	case true:
		return Err.ErrStateAlreadyMet
	default:
		return r.AddFunc(r.Username)
	}
}

func (r *AddUserApplicator) Revert(ctx context.Context) error {

	// Removing a user is hard to restor as it usually will
	// orphan a lot of resources. For now we return the no-op
	// error type. Revert can be call but will not revert anything
	return Err.ErrNoOp
}

func DefaultAddFunc(uname string) error {
	return userutil.Useradd(uname)
}
