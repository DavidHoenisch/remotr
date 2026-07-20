// Package groups applies local group resources through fixed argv commands.
package groups

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator converges one local group under the account-database lock.
type Applicator struct {
	Resource models.GroupResource
	Runner   executil.Runner
}

func New(resource models.GroupResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner}
}

func (a *Applicator) Name() string { return "group:" + a.Resource.Name }

func (a *Applicator) Description() string { return "group " + a.Resource.Group }

func (a *Applicator) State(_ context.Context) (any, bool) {
	group, err := a.lookup()
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return group, err != nil
	}
	if err != nil {
		return nil, false
	}
	if a.Resource.GID > 0 && group.GID != a.Resource.GID {
		return group, false
	}
	if a.Resource.System != nil && isSystemGID(group.GID) != *a.Resource.System {
		return group, false
	}
	return group, true
}

func (a *Applicator) Apply(_ context.Context) error {
	group, lookupErr := a.lookup()
	if errors.Is(lookupErr, errInvalidGroupLookup) {
		return lookupErr
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if lookupErr != nil {
			return appErr.ErrStateAlreadyMet
		}
		_, _, err := a.Runner.Run("groupdel", "--", a.Resource.Group)
		return err
	}
	if lookupErr != nil {
		args := make([]string, 0, 5)
		if a.Resource.System != nil && *a.Resource.System {
			args = append(args, "--system")
		}
		if a.Resource.GID > 0 {
			args = append(args, "--gid", strconv.Itoa(a.Resource.GID))
		}
		args = append(args, "--", a.Resource.Group)
		_, _, err := a.Runner.Run("groupadd", args...)
		return err
	}
	if a.Resource.GID == 0 && (a.Resource.System == nil || isSystemGID(group.GID) == *a.Resource.System) {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.GID > 0 && group.GID != a.Resource.GID {
		if !a.Resource.AllowGIDReassignment {
			return fmt.Errorf("group %q gid reassignment from %d to %d requires allowGIDReassignment", a.Resource.Group, group.GID, a.Resource.GID)
		}
		_, _, err := a.Runner.Run("groupmod", "--gid", strconv.Itoa(a.Resource.GID), "--", a.Resource.Group)
		return err
	}
	if a.Resource.System != nil && isSystemGID(group.GID) == *a.Resource.System {
		return appErr.ErrStateAlreadyMet
	}
	return fmt.Errorf("group %q system class cannot be changed without a gid reassignment", a.Resource.Group)
}

func (*Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

type observedGroup struct {
	Name string
	GID  int
}

var errInvalidGroupLookup = errors.New("invalid group lookup result")

func (a *Applicator) lookup() (*observedGroup, error) {
	stdout, _, err := a.Runner.Run("getent", "group", a.Resource.Group)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSpace(string(stdout)), ":")
	if len(fields) < 3 || fields[0] != a.Resource.Group {
		return nil, fmt.Errorf("%w for %q", errInvalidGroupLookup, a.Resource.Group)
	}
	gid, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("%w for %q gid: %v", errInvalidGroupLookup, a.Resource.Group, err)
	}
	return &observedGroup{Name: fields[0], GID: gid}, nil
}

func isSystemGID(gid int) bool { return gid > 0 && gid < 1000 }
