package files

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"golang.org/x/sys/unix"
)

// Owner sets POSIX ownership after writing a file (optional).
type Owner struct {
	UID int
	GID int
}

type Applicator struct {
	File     models.File
	Owner    *Owner
	SafeBase string
	rollback *rollbackstore.Handle
	previous rollbackSnapshot
	armed    bool
}

type rollbackSnapshot struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Content []byte `json:"content,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
	UID     int    `json:"uid,omitempty"`
	GID     int    `json:"gid,omitempty"`
}

func New(f models.File) *Applicator {
	return &Applicator{File: f, SafeBase: string(os.PathSeparator)}
}

// ConfigureRollback binds this provider to the agent transaction store.
func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := rollbackstore.NewHandle(store, address, artifactDigest, false)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
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

func (a *Applicator) Apply(ctx context.Context) error {
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
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := a.captureRollback(path)
	if err != nil {
		return err
	}
	if a.File.Lifecycle == models.LifecycleAbsent {
		if err := a.armRollback(ctx, previous); err != nil {
			return err
		}
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
		if previous.Existed && a.contentMet(previous.Content) {
			mode := os.FileMode(0o644)
			if len(a.File.Mode) > 0 {
				mode = os.FileMode(a.File.Mode[0] & 0o777)
			} else if info, statErr := os.Stat(path); statErr == nil {
				mode = info.Mode().Perm()
			}
			if err := a.armRollback(ctx, previous); err != nil {
				return err
			}
			if err := os.Chmod(path, mode); err != nil {
				return err
			}
			return a.chown(path)
		}
	}
	existing := previous.Content
	body, err := a.applyBody(string(existing))
	if err != nil {
		return err
	}
	if err := a.armRollback(ctx, previous); err != nil {
		return err
	}
	if a.SafeBase != "" {
		mode := os.FileMode(0o644)
		if len(a.File.Mode) > 0 {
			mode = os.FileMode(a.File.Mode[0] & 0o777)
		}
		return a.safeWrite(path, body, mode, a.Owner)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if len(a.File.Mode) > 0 {
		mode = os.FileMode(a.File.Mode[0] & 0o777)
	}
	if err := atomicWriteFile(path, body, mode, a.Owner); err != nil {
		return err
	}
	return nil
}

// ApplyResult advertises durable rollback only when the registry supplied an
// agent transaction handle. Internal file helpers without that handle remain
// honest about not offering a restart-safe rollback guarantee.
func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
}

func (a *Applicator) captureRollback(path string) (rollbackSnapshot, error) {
	content, existed, err := a.safeReadExisting(path)
	if a.SafeBase == "" {
		content, err = os.ReadFile(path) // #nosec G304 -- absolute path validated above.
		existed = err == nil
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return rollbackSnapshot{}, fmt.Errorf("capture protected file rollback state: %w", err)
	}
	snapshot := rollbackSnapshot{Version: 1, Path: path, Existed: existed, Content: append([]byte(nil), content...)}
	if !existed {
		return snapshot, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return rollbackSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return rollbackSnapshot{}, fmt.Errorf("managed file rollback target must be regular")
	}
	snapshot.Mode = uint32(info.Mode().Perm())
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		snapshot.UID, snapshot.GID = int(stat.Uid), int(stat.Gid)
	}
	return snapshot, nil
}

func (a *Applicator) armRollback(ctx context.Context, snapshot rollbackSnapshot) error {
	if a.rollback == nil {
		a.previous = snapshot
		a.armed = true
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	defer clear(payload)
	if err := a.rollback.Arm(ctx, payload); err != nil {
		return fmt.Errorf("arm protected file rollback: %w", err)
	}
	return nil
}

func (a *Applicator) restoreRollback(path string, snapshot rollbackSnapshot) error {
	if snapshot.Version != 1 || snapshot.Path != path {
		return errors.New("protected file rollback identity is invalid")
	}
	if !snapshot.Existed {
		if a.SafeBase != "" {
			if err := a.safeRemove(path); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ENOENT) {
				return err
			}
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	mode := os.FileMode(snapshot.Mode).Perm()
	owner := &Owner{UID: snapshot.UID, GID: snapshot.GID}
	if a.SafeBase != "" {
		return a.safeWrite(path, snapshot.Content, mode, owner)
	}
	return atomicWriteFile(path, snapshot.Content, mode, owner)
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

func (a *Applicator) safeWrite(path string, body []byte, mode os.FileMode, owner *Owner) error {
	rel, err := a.safeRelative(path)
	if err != nil {
		return err
	}
	parent, name, err := openSafeParent(a.SafeBase, rel, true)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	return writeAt(parent, name, body, mode, owner)
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

func (a *Applicator) Revert(ctx context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if a.rollback != nil {
		return a.rollback.Rollback(ctx, func(payload []byte) error {
			var snapshot rollbackSnapshot
			if err := json.Unmarshal(payload, &snapshot); err != nil {
				return err
			}
			return a.restoreRollback(path, snapshot)
		})
	}
	if !a.armed {
		return appErr.ErrNoOp
	}
	err = a.restoreRollback(path, a.previous)
	a.previous = rollbackSnapshot{}
	a.armed = false
	return err
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
