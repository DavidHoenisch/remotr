package remove

import (
	"context"
	"errors"
	"log"
	"os/user"

	Err "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/userutil"
)

type RemoveUserApplicator struct {
	Username   string
	Uid        int
	RemoveFunc func(uname string) error
}

func (r *RemoveUserApplicator) Name() string { return "RemoveUserApplicator" }

func (r *RemoveUserApplicator) Description() string { return "Remove a user account from system" }

func (r *RemoveUserApplicator) State(ctx context.Context) (any, bool) {
	u, err := user.Lookup(r.Username)

	if errors.As(err, new(user.UnknownUserError)) {
		return nil, true
	}

	log.Printf("User exists as %s", u.Uid)

	return u, false
}

func (r *RemoveUserApplicator) Apply(ctx context.Context) error {
	_, met := r.State(ctx)
	if met {
		return Err.ErrStateAlreadyMet
	}
	return r.RemoveFunc(r.Username)
}

func (r *RemoveUserApplicator) Revert(ctx context.Context) error {

	// Removing a user is hard to restor as it usually will
	// orphan a lot of resources. For now we return the no-op
	// error type. Revert can be call but will not revert anything
	return Err.ErrNoOp
}

func DefaultRemoveFunc(uname string) error {
	return userutil.Userdel(uname)
}
