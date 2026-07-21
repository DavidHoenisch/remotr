// Package desktopstate validates interactive-user desktop state paths before
// providers invoke tools that may create or update files below a home directory.
package desktopstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/fsops"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"golang.org/x/sys/unix"
)

// ValidateUserPath rejects invalid homes and symlink traversal through the
// parent directories of the relative desktop state path. Missing state
// directories are allowed because native desktop tools create them as needed.
func ValidateUserPath(user interactiveuser.Account, relativeStatePath string) error {
	home := filepath.Clean(strings.TrimSpace(user.HomeDir))
	if !filepath.IsAbs(home) || home == string(os.PathSeparator) || home != user.HomeDir {
		return fmt.Errorf("user %s home path is invalid", user.Username)
	}
	relativeStatePath = filepath.Clean(strings.TrimSpace(relativeStatePath))
	if relativeStatePath == "." || filepath.IsAbs(relativeStatePath) || relativeStatePath == ".." || strings.HasPrefix(relativeStatePath, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("user %s desktop state path is invalid", user.Username)
	}
	fd, _, err := fsops.OpenSafeParent(filepath.Join(home, ".remotr-desktop-state-home"), false)
	if err != nil {
		return fmt.Errorf("user %s home path is not a safe directory: %w", user.Username, err)
	}
	_ = unix.Close(fd)

	fd, _, err = fsops.OpenSafeParent(filepath.Join(home, relativeStatePath), false)
	if err == nil {
		_ = unix.Close(fd)
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("user %s desktop state path is unsafe: %w", user.Username, err)
}
