// Package directories applies bounded, single-directory resources.
package directories

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/applicators/fsops"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"golang.org/x/sys/unix"
)

// Applicator converges a single directory. It walks parent components with
// O_NOFOLLOW so an untrusted symlink cannot redirect a mutation.
type Applicator struct {
	Directory models.DirectoryResource
}

func New(directory models.DirectoryResource) *Applicator {
	return &Applicator{Directory: directory}
}

func (a *Applicator) Name() string { return "directory:" + a.Directory.Name }

func (a *Applicator) Description() string { return "directory " + a.Directory.Path }

func (a *Applicator) State(_ context.Context) (any, bool) {
	parent, name, err := fsops.OpenSafeParent(a.Directory.Path, false)
	if err != nil {
		return nil, false
	}
	defer unix.Close(parent)

	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if a.Directory.Lifecycle == models.LifecycleAbsent {
		return nil, err == unix.ENOENT
	}
	if err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, false
	}
	owner, err := fsops.ResolveOwnership(a.Directory.Owner, a.Directory.Group)
	if err != nil {
		return nil, false
	}
	if len(a.Directory.Mode) > 0 && stat.Mode&0o777 != uint32(a.Directory.Mode[0]&0o777) {
		return nil, false
	}
	if owner.UID >= 0 && int(stat.Uid) != owner.UID {
		return nil, false
	}
	if owner.GID >= 0 && int(stat.Gid) != owner.GID {
		return nil, false
	}
	return nil, true
}

func (a *Applicator) Apply(_ context.Context) error {
	parent, name, err := fsops.OpenSafeParent(a.Directory.Path, a.Directory.Lifecycle != models.LifecycleAbsent)
	if err != nil {
		return err
	}
	defer unix.Close(parent)

	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if a.Directory.Lifecycle == models.LifecycleAbsent {
		if err == unix.ENOENT {
			return appErr.ErrStateAlreadyMet
		}
		if err != nil {
			return err
		}
		flags := 0
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			flags = unix.AT_REMOVEDIR
		}
		return unix.Unlinkat(parent, name, flags)
	}
	if err != nil && err != unix.ENOENT {
		return err
	}
	if err == unix.ENOENT {
		if err := unix.Mkdirat(parent, name, uint32(fsops.DesiredMode(a.Directory.Mode, 0o755))); err != nil {
			return err
		}
	} else if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		if !a.Directory.AllowTypeReplacement {
			return fmt.Errorf("refusing to replace non-directory %q without allowTypeReplacement", a.Directory.Path)
		}
		if err := unix.Unlinkat(parent, name, 0); err != nil {
			return err
		}
		if err := unix.Mkdirat(parent, name, uint32(fsops.DesiredMode(a.Directory.Mode, 0o755))); err != nil {
			return err
		}
	}

	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if len(a.Directory.Mode) > 0 {
		if err := unix.Fchmod(fd, uint32(fsops.DesiredMode(a.Directory.Mode, 0o755))); err != nil {
			return err
		}
	}
	owner, err := fsops.ResolveOwnership(a.Directory.Owner, a.Directory.Group)
	if err != nil {
		return err
	}
	if owner.UID >= 0 || owner.GID >= 0 {
		if err := unix.Fchown(fd, owner.UID, owner.GID); err != nil {
			return err
		}
	}
	return nil
}

func (*Applicator) Revert(context.Context) error { return appErr.ErrNoOp }
