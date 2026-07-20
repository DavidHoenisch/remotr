// Package links applies symbolic and hard link resources without following
// untrusted parent or source path components.
package links

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/applicators/fsops"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"golang.org/x/sys/unix"
)

// Applicator converges one link resource.
type Applicator struct{ Link models.LinkResource }

func New(link models.LinkResource) *Applicator { return &Applicator{Link: link} }

func (a *Applicator) Name() string { return "link:" + a.Link.Name }

func (a *Applicator) Description() string { return "link " + a.Link.Path }

func (a *Applicator) State(_ context.Context) (any, bool) {
	parent, name, err := fsops.OpenSafeParent(a.Link.Path, false)
	if err != nil {
		return nil, false
	}
	defer unix.Close(parent)
	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if a.Link.Lifecycle == models.LifecycleAbsent {
		return nil, err == unix.ENOENT
	}
	if err != nil {
		return nil, false
	}
	switch a.Link.LinkType {
	case models.LinkTypeSymbolic:
		if stat.Mode&unix.S_IFMT != unix.S_IFLNK {
			return nil, false
		}
		target, err := readlinkAt(parent, name)
		if err != nil || target != a.Link.Target {
			return nil, false
		}
		owner, err := fsops.ResolveOwnership(a.Link.Owner, a.Link.Group)
		if err != nil {
			return nil, false
		}
		return nil, (owner.UID < 0 || int(stat.Uid) == owner.UID) && (owner.GID < 0 || int(stat.Gid) == owner.GID)
	case models.LinkTypeHard:
		if stat.Mode&unix.S_IFMT != unix.S_IFREG {
			return nil, false
		}
		source, err := fsops.OpenRegularNoFollow(a.Link.Target)
		if err != nil {
			return nil, false
		}
		defer unix.Close(source)
		var sourceStat unix.Stat_t
		if err := unix.Fstat(source, &sourceStat); err != nil {
			return nil, false
		}
		return nil, stat.Dev == sourceStat.Dev && stat.Ino == sourceStat.Ino
	default:
		return nil, false
	}
}

func (a *Applicator) Apply(_ context.Context) error {
	owner := fsops.Ownership{UID: -1, GID: -1}
	source := -1
	if a.Link.Lifecycle != models.LifecycleAbsent && a.Link.LinkType == models.LinkTypeSymbolic {
		var err error
		owner, err = fsops.ResolveOwnership(a.Link.Owner, a.Link.Group)
		if err != nil {
			return err
		}
	}
	if a.Link.Lifecycle != models.LifecycleAbsent && a.Link.LinkType == models.LinkTypeHard {
		var err error
		source, err = fsops.OpenRegularNoFollow(a.Link.Target)
		if err != nil {
			return err
		}
		defer unix.Close(source)
	}
	parent, name, err := fsops.OpenSafeParent(a.Link.Path, a.Link.Lifecycle != models.LifecycleAbsent)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if a.Link.Lifecycle == models.LifecycleAbsent {
		if err == unix.ENOENT {
			return appErr.ErrStateAlreadyMet
		}
		if err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refusing to remove directory %q through link resource", a.Link.Path)
		}
		return unix.Unlinkat(parent, name, 0)
	}
	if err != nil && err != unix.ENOENT {
		return err
	}
	if err == nil {
		_, met := a.State(context.Background())
		if met {
			return appErr.ErrStateAlreadyMet
		}
		if !a.Link.AllowTypeReplacement {
			return fmt.Errorf("refusing to replace existing object %q without allowTypeReplacement", a.Link.Path)
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refusing to replace directory %q through link resource", a.Link.Path)
		}
		if err := unix.Unlinkat(parent, name, 0); err != nil {
			return err
		}
	}

	switch a.Link.LinkType {
	case models.LinkTypeSymbolic:
		if err := unix.Symlinkat(a.Link.Target, parent, name); err != nil {
			return err
		}
		if owner.UID >= 0 || owner.GID >= 0 {
			return unix.Fchownat(parent, name, owner.UID, owner.GID, unix.AT_SYMLINK_NOFOLLOW)
		}
		return nil
	case models.LinkTypeHard:
		return unix.Linkat(source, "", parent, name, unix.AT_EMPTY_PATH)
	default:
		return fmt.Errorf("unsupported link type %q", a.Link.LinkType)
	}
}

func (*Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func readlinkAt(parent int, name string) (string, error) {
	buf := make([]byte, 4096)
	n, err := unix.Readlinkat(parent, name, buf)
	if err != nil {
		return "", err
	}
	if n == len(buf) {
		return "", fmt.Errorf("symbolic link target is too long")
	}
	return string(buf[:n]), nil
}
