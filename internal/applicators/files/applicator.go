package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
	"golang.org/x/sys/unix"
)

const backupSuffix = ".remotr.bak"

// Owner sets POSIX ownership after writing a file (optional).
type Owner struct {
	UID int
	GID int
}

type Applicator struct {
	File     models.File
	Owner    *Owner
	SafeBase string
}

func New(f models.File) *Applicator {
	return &Applicator{File: f, SafeBase: string(os.PathSeparator)}
}

// NewOwned returns an applicator that chowns the path to uid/gid after apply and revert.
func NewOwned(f models.File, uid, gid int) *Applicator {
	return &Applicator{
		File:  f,
		Owner: &Owner{UID: uid, GID: gid},
	}
}

// NewOwnedUnder returns an owned applicator that refuses to follow symlinks
// while applying files under base. This is intended for user-writable trees.
func NewOwnedUnder(f models.File, base string, uid, gid int) *Applicator {
	return &Applicator{
		File:     f,
		Owner:    &Owner{UID: uid, GID: gid},
		SafeBase: base,
	}
}

func (a *Applicator) Name() string { return "file:" + a.File.Name }

func (a *Applicator) Description() string { return "file " + a.File.Path }

func (a *Applicator) path() (string, error) {
	return validateAbsPath(a.File.Path)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	path, err := a.path()
	if err != nil {
		return nil, false
	}
	if a.File.Lifecycle == models.LifecycleAbsent {
		_, err := os.Lstat(path)
		return nil, os.IsNotExist(err)
	}
	var content []byte
	if a.SafeBase != "" {
		content, err = a.safeRead(path)
	} else {
		content, err = os.ReadFile(path) // #nosec G304 -- absolute path validated
	}
	if err != nil {
		if os.IsNotExist(err) {
			if a.File.Content != "" || strings.TrimSpace(a.File.WithRegx) != "" {
				return nil, false
			}
			return nil, true
		}
		return nil, false
	}
	if a.File.WithRegx != "" {
		re, err := regexp.Compile(a.File.WithRegx)
		if err != nil {
			return string(content), false
		}
		return string(content), re.Match(content) && a.metadataMet(path)
	}
	if a.File.Content != "" {
		return string(content), string(content) == a.File.Content && a.metadataMet(path)
	}
	return string(content), a.metadataMet(path)
}

func (a *Applicator) metadataMet(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if len(a.File.Mode) > 0 && info.Mode().Perm() != os.FileMode(a.File.Mode[0]&0o777) {
		return false
	}
	owner, err := a.desiredOwner()
	if err != nil {
		return false
	}
	if owner != nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (owner.UID >= 0 && int(stat.Uid) != owner.UID) || (owner.GID >= 0 && int(stat.Gid) != owner.GID) {
			return false
		}
	}
	return true
}

func (a *Applicator) Apply(_ context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	owner, err := a.desiredOwner()
	if err != nil {
		return err
	}
	if owner != nil {
		a.Owner = owner
	}
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.File.Lifecycle == models.LifecycleAbsent {
		if a.SafeBase != "" {
			rel, err := a.safeRelative(path)
			if err != nil {
				return err
			}
			parent, name, err := openSafeParent(a.SafeBase, rel, false)
			if err != nil {
				return err
			}
			defer unix.Close(parent)
			if err := unix.Unlinkat(parent, name, 0); err != nil {
				return err
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to remove non-regular file %s", path)
		}
		return os.Remove(path)
	}
	if a.SafeBase == "" {
		if content, readErr := os.ReadFile(path); readErr == nil && a.contentMet(content) {
			mode := os.FileMode(0o644)
			if len(a.File.Mode) > 0 {
				mode = os.FileMode(a.File.Mode[0] & 0o777)
			} else if info, statErr := os.Stat(path); statErr == nil {
				mode = info.Mode().Perm()
			}
			if err := os.Chmod(path, mode); err != nil {
				return err
			}
			return a.chown(path)
		}
	}
	if a.SafeBase != "" {
		return a.applySafe(path)
	}
	bak := path + backupSuffix
	var existing []byte
	if _, err := os.Stat(path); err == nil {
		existing, err = os.ReadFile(path) // #nosec G304
		if err != nil {
			return err
		}
		if err := os.WriteFile(bak, existing, 0o600); err != nil { // #nosec G703 -- path validated absolute
			return fmt.Errorf("backup %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if len(a.File.Mode) > 0 {
		mode = os.FileMode(a.File.Mode[0] & 0o777)
	}
	body, err := a.applyBody(string(existing))
	if err != nil {
		return err
	}
	if err := atomicWriteFile(path, body, mode, a.Owner); err != nil {
		return err
	}
	return nil
}

func (a *Applicator) desiredOwner() (*Owner, error) {
	if a.Owner != nil {
		return a.Owner, nil
	}
	if a.File.Owner == "" && a.File.Group == "" {
		return nil, nil
	}
	result := &Owner{UID: -1, GID: -1}
	if a.File.Owner != "" {
		u, err := user.Lookup(a.File.Owner)
		if err != nil {
			return nil, fmt.Errorf("owner %q: %w", a.File.Owner, err)
		}
		uid, err := strconv.Atoi(u.Uid)
		if err != nil {
			return nil, err
		}
		result.UID = uid
	}
	if a.File.Group != "" {
		g, err := user.LookupGroup(a.File.Group)
		if err != nil {
			return nil, fmt.Errorf("group %q: %w", a.File.Group, err)
		}
		gid, err := strconv.Atoi(g.Gid)
		if err != nil {
			return nil, err
		}
		result.GID = gid
	}
	return result, nil
}

func (a *Applicator) contentMet(content []byte) bool {
	if a.File.WithRegx != "" {
		re, err := regexp.Compile(a.File.WithRegx)
		return err == nil && re.Match(content)
	}
	if a.File.Content != "" {
		return string(content) == a.File.Content
	}
	return true
}

func (a *Applicator) applySafe(path string) error {
	existing, existed, err := a.safeReadExisting(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if len(a.File.Mode) > 0 {
		mode = os.FileMode(a.File.Mode[0] & 0o777)
	}
	body, err := a.applyBody(string(existing))
	if err != nil {
		return err
	}
	return a.safeWrite(path, body, mode, existing, existed)
}

func (a *Applicator) safeRelative(path string) (string, error) {
	base := filepath.Clean(a.SafeBase)
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("file path escapes safe base")
	}
	return rel, nil
}

func (a *Applicator) safeRead(path string) ([]byte, error) {
	data, existed, err := a.safeReadExisting(path)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, os.ErrNotExist
	}
	return data, nil
}

// ReadOwnedUnder reads an absolute path below base using no-follow traversal.
// It is shared by structured per-user resources that must inspect existing
// state without trusting user-writable parent directories.
func ReadOwnedUnder(base, path string) ([]byte, bool, error) {
	a := &Applicator{SafeBase: base}
	return a.safeReadExisting(path)
}

func (a *Applicator) safeReadExisting(path string) ([]byte, bool, error) {
	rel, err := a.safeRelative(path)
	if err != nil {
		return nil, false, err
	}
	parent, name, err := openSafeParent(a.SafeBase, rel, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ENOENT {
			return nil, false, nil
		}
		return nil, false, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (a *Applicator) safeWrite(path string, body []byte, mode os.FileMode, existing []byte, existed bool) error {
	rel, err := a.safeRelative(path)
	if err != nil {
		return err
	}
	parent, name, err := openSafeParent(a.SafeBase, rel, true)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if existed {
		if err := writeAt(parent, name+backupSuffix, existing, 0o600, nil); err != nil {
			return fmt.Errorf("backup %s: %w", path, err)
		}
	}
	return writeAt(parent, name, body, mode, a.Owner)
}

func openSafeParent(base, rel string, create bool) (int, string, error) {
	parts := strings.Split(filepath.Clean(rel), string(os.PathSeparator))
	if len(parts) == 0 || parts[0] == "." || parts[len(parts)-1] == "" {
		return -1, "", fmt.Errorf("invalid file path")
	}
	fd, err := unix.Open(filepath.Clean(base), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return -1, "", fmt.Errorf("invalid file path")
		}
		if create {
			if err := unix.Mkdirat(fd, part, 0o750); err != nil && err != unix.EEXIST {
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

func writeAt(parent int, name string, body []byte, mode os.FileMode, owner *Owner) error {
	tmpName := fmt.Sprintf(".remotr-%s-%d.tmp", name, os.Getpid())
	fd, err := unix.Openat(parent, tmpName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = unix.Close(fd)
		if cleanup {
			_ = unix.Unlinkat(parent, tmpName, 0)
		}
	}()
	if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
		return err
	}
	if owner != nil {
		if err := unix.Fchown(fd, owner.UID, owner.GID); err != nil {
			return err
		}
	}
	for len(body) > 0 {
		n, err := unix.Write(fd, body)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		body = body[n:]
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	if err := unix.Close(fd); err != nil {
		fd = -1
		return err
	}
	fd = -1
	if err := unix.Renameat(parent, tmpName, parent, name); err != nil {
		return err
	}
	cleanup = false
	return unix.Fsync(parent)
}

func atomicWriteFile(path string, body []byte, mode os.FileMode, owner *Owner) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".remotr-file-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if owner != nil {
		if err := tmp.Chown(owner.UID, owner.GID); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (a *Applicator) chown(path string) error {
	if a.Owner == nil {
		return nil
	}
	return os.Chown(path, a.Owner.UID, a.Owner.GID)
}

func (a *Applicator) applyBody(existing string) ([]byte, error) {
	if a.File.UpdateExisting && strings.TrimSpace(a.File.WithRegx) != "" {
		lineRe, err := lineReplacePattern(a.File)
		if err != nil {
			return nil, err
		}
		updated, _, err := applyLineEdit(existing, lineRe, a.File.Content)
		if err != nil {
			return nil, err
		}
		return []byte(updated), nil
	}
	if a.File.Content == "" {
		return nil, fmt.Errorf("file %q: content required", a.File.Name)
	}
	return []byte(a.File.Content), nil
}

func (a *Applicator) Revert(_ context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if a.SafeBase != "" {
		return a.revertSafe(path)
	}
	bak := path + backupSuffix
	data, err := os.ReadFile(bak) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return os.Remove(path)
		}
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- restore prior content, validated path
		return err
	}
	if err := a.chown(path); err != nil {
		return err
	}
	return os.Remove(bak)
}

func (a *Applicator) revertSafe(path string) error {
	bak := path + backupSuffix
	data, err := a.safeRead(bak)
	if err != nil {
		if os.IsNotExist(err) {
			return a.safeRemove(path)
		}
		return err
	}
	if err := a.safeWrite(path, data, 0o644, nil, false); err != nil {
		return err
	}
	return a.safeRemove(bak)
}

func (a *Applicator) safeRemove(path string) error {
	rel, err := a.safeRelative(path)
	if err != nil {
		return err
	}
	parent, name, err := openSafeParent(a.SafeBase, rel, false)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	return unix.Unlinkat(parent, name, 0)
}

func validateAbsPath(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return "", fmt.Errorf("file path is required")
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("file path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid file path")
	}
	return clean, nil
}
