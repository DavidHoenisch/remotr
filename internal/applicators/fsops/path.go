// Package fsops contains no-follow filesystem primitives shared by managed
// filesystem object providers.
package fsops

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Ownership contains independently optional POSIX owner and group IDs.
type Ownership struct {
	UID int
	GID int
}

// ResolveOwnership resolves declared account names without treating omitted
// owner or group as an instruction to modify that property.
func ResolveOwnership(ownerName, groupName string) (Ownership, error) {
	result := Ownership{UID: -1, GID: -1}
	if ownerName != "" {
		entry, err := user.Lookup(ownerName)
		if err != nil {
			return Ownership{}, fmt.Errorf("owner %q: %w", ownerName, err)
		}
		uid, err := strconv.Atoi(entry.Uid)
		if err != nil {
			return Ownership{}, err
		}
		result.UID = uid
	}
	if groupName != "" {
		entry, err := user.LookupGroup(groupName)
		if err != nil {
			return Ownership{}, fmt.Errorf("group %q: %w", groupName, err)
		}
		gid, err := strconv.Atoi(entry.Gid)
		if err != nil {
			return Ownership{}, err
		}
		result.GID = gid
	}
	return result, nil
}

// DesiredMode returns the declared mode or the resource default.
func DesiredMode(values []int, fallback os.FileMode) os.FileMode {
	if len(values) == 0 {
		return fallback
	}
	return os.FileMode(values[0] & 0o777)
}

// OpenSafeParent opens the parent directory for path without following any
// component symlink. When create is true, missing parent directories are
// created with a conservative mode before they are opened again no-follow.
func OpenSafeParent(path string, create bool) (int, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) || clean == string(os.PathSeparator) {
		return -1, "", fmt.Errorf("filesystem path must be an absolute non-root path")
	}
	parts := strings.Split(strings.TrimPrefix(clean, string(os.PathSeparator)), string(os.PathSeparator))
	fd, err := unix.Open(string(os.PathSeparator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return -1, "", fmt.Errorf("invalid filesystem path")
		}
		if create {
			if err := unix.Mkdirat(fd, part, 0o755); err != nil && err != unix.EEXIST {
				unix.Close(fd)
				return -1, "", err
			}
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			return -1, "", err
		}
		fd = next
	}
	return fd, parts[len(parts)-1], nil
}

// OpenRegularNoFollow opens a regular hard-link source only after confirming
// every path component is non-symlinked.
func OpenRegularNoFollow(path string) (int, error) {
	parent, name, err := OpenSafeParent(path, false)
	if err != nil {
		return -1, err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return -1, fmt.Errorf("hard-link target %q is not a regular file", path)
	}
	return fd, nil
}
