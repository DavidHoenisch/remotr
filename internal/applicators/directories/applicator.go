// Package directories applies bounded, single-directory resources.
package directories

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

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
	if a.Directory.Purge {
		fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, false
		}
		defer unix.Close(fd)
		needed, err := a.purgeNeeded(fd)
		if err != nil || needed {
			return nil, false
		}
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
	if a.Directory.Purge {
		if err := a.purge(fd); err != nil {
			return err
		}
	}
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

func (a *Applicator) purgeNeeded(root int) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil {
		return false, err
	}
	return a.scan(root, stat.Dev, "", 0, new(int))
}

func (a *Applicator) scan(dir int, rootDev uint64, relative string, depth int, entries *int) (bool, error) {
	names, err := readNames(dir)
	if err != nil {
		return false, err
	}
	needed := false
	for _, name := range names {
		childRelative := path.Join(relative, name)
		if a.excluded(childRelative) {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(dir, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return false, err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			if !a.Directory.CrossFilesystem && stat.Dev != rootDev {
				continue
			}
			if depth >= a.Directory.MaxDepth {
				return false, fmt.Errorf("directory %q exceeds maxDepth %d", a.Directory.Path, a.Directory.MaxDepth)
			}
			child, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return false, err
			}
			childNeeded, err := a.scan(child, rootDev, childRelative, depth+1, entries)
			unix.Close(child)
			if err != nil {
				return false, err
			}
			needed = needed || childNeeded
		}
		*entries++
		if *entries > a.Directory.MaxEntries {
			return false, fmt.Errorf("directory %q exceeds maxEntries %d", a.Directory.Path, a.Directory.MaxEntries)
		}
		needed = true
	}
	return needed, nil
}

func (a *Applicator) purge(root int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil {
		return err
	}
	needed, err := a.scan(root, stat.Dev, "", 0, new(int))
	if err != nil || !needed {
		return err
	}
	return a.removeChildren(root, stat.Dev, "", 0)
}

func (a *Applicator) removeChildren(dir int, rootDev uint64, relative string, depth int) error {
	names, err := readNames(dir)
	if err != nil {
		return err
	}
	for _, name := range names {
		childRelative := path.Join(relative, name)
		if a.excluded(childRelative) {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(dir, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			if !a.Directory.CrossFilesystem && stat.Dev != rootDev {
				continue
			}
			if depth >= a.Directory.MaxDepth {
				return fmt.Errorf("directory %q exceeds maxDepth %d", a.Directory.Path, a.Directory.MaxDepth)
			}
			child, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			err = a.removeChildren(child, rootDev, childRelative, depth+1)
			unix.Close(child)
			if err != nil {
				return err
			}
			if err := unix.Unlinkat(dir, name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
			continue
		}
		if err := unix.Unlinkat(dir, name, 0); err != nil {
			return err
		}
	}
	return nil
}

func (a *Applicator) excluded(relative string) bool {
	for _, pattern := range a.Directory.Exclusions {
		if matched, err := path.Match(pattern, relative); err == nil && matched {
			return true
		}
	}
	return false
}

func readNames(fd int) ([]string, error) {
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	if _, err := unix.Seek(duplicate, 0, io.SeekStart); err != nil {
		unix.Close(duplicate)
		return nil, err
	}
	directory := os.NewFile(uintptr(duplicate), "directory")
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}
