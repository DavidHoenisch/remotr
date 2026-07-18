// Package filetx provides path-bound protected rollback snapshots for
// applicators that own one or more regular files.
package filetx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const snapshotVersion = 1

type snapshot struct {
	Version int     `json:"version"`
	Entries []entry `json:"entries"`
}

type entry struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content []byte `json:"content,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
	UID     int    `json:"uid,omitempty"`
	GID     int    `json:"gid,omitempty"`
}

// Handle protects complete snapshots through the agent rollback store.
type Handle struct {
	transaction *rollbackstore.Handle
}

// New binds a fixed resource identity to a protected multi-file transaction.
func New(store *rollbackstore.Store, address, artifactDigest string, sensitive bool) (*Handle, error) {
	transaction, err := rollbackstore.NewHandle(store, address, artifactDigest, sensitive)
	if err != nil {
		return nil, err
	}
	return &Handle{transaction: transaction}, nil
}

// Arm captures every managed path and persists the complete snapshot before
// the provider mutates any of them.
func (h *Handle) Arm(ctx context.Context, paths ...string) error {
	if h == nil || h.transaction == nil {
		return errors.New("file transaction handle is required")
	}
	if len(paths) == 0 {
		return errors.New("file transaction requires at least one managed path")
	}
	snapshot := snapshot{Version: snapshotVersion, Entries: make([]entry, 0, len(paths))}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if !filepath.IsAbs(clean) || clean != path {
			return fmt.Errorf("file transaction path %q must be clean and absolute", path)
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("file transaction path %q is duplicated", clean)
		}
		seen[clean] = struct{}{}
		captured, err := capture(clean)
		if err != nil {
			return err
		}
		snapshot.Entries = append(snapshot.Entries, captured)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	defer clear(payload)
	if err := h.transaction.Arm(ctx, payload); err != nil {
		return fmt.Errorf("arm protected file transaction: %w", err)
	}
	return nil
}

// Rollback restores the path-bound snapshot and completes its durable
// transaction. It works with a reconstructed Handle after process restart.
func (h *Handle) Rollback(ctx context.Context) error {
	if h == nil || h.transaction == nil {
		return os.ErrNotExist
	}
	return h.transaction.Rollback(ctx, func(payload []byte) error {
		var snapshot snapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return err
		}
		if snapshot.Version != snapshotVersion || len(snapshot.Entries) == 0 {
			return errors.New("protected file transaction snapshot is invalid")
		}
		for _, captured := range snapshot.Entries {
			defer clear(captured.Content)
			if !filepath.IsAbs(captured.Path) || filepath.Clean(captured.Path) != captured.Path {
				return errors.New("protected file transaction path is invalid")
			}
			if err := restore(captured); err != nil {
				return err
			}
		}
		return nil
	})
}

func capture(path string) (entry, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return entry{Path: path, UID: -1, GID: -1}, nil
	}
	if err != nil {
		return entry{}, err
	}
	if !info.Mode().IsRegular() {
		return entry{}, fmt.Errorf("managed rollback path %q must be a regular file", path)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- path is validated and provider-owned.
	if err != nil {
		return entry{}, err
	}
	uid, gid := -1, -1
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid, gid = int(stat.Uid), int(stat.Gid)
	}
	return entry{Path: path, Exists: true, Content: content, Mode: uint32(info.Mode().Perm()), UID: uid, GID: gid}, nil
}

func restore(captured entry) error {
	if !captured.Exists {
		if err := os.Remove(captured.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(captured.Path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(captured.Path), ".remotr-restore-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(os.FileMode(captured.Mode).Perm()); err != nil {
		return err
	}
	if captured.UID >= 0 || captured.GID >= 0 {
		if err := temporary.Chown(captured.UID, captured.GID); err != nil {
			return err
		}
	}
	if _, err := temporary.Write(captured.Content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, captured.Path); err != nil {
		return err
	}
	cleanup = false
	directory, err := os.Open(filepath.Dir(captured.Path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
