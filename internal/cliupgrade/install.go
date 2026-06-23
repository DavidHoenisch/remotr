package cliupgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func installBinary(data []byte, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	staging := dest + ".new"
	if err := os.WriteFile(staging, data, 0o755); err != nil { // #nosec G306 G703
		return fmt.Errorf("stage binary: %w", err)
	}
	if err := os.Rename(staging, dest); err != nil {
		_ = os.Remove(staging)
		return fmt.Errorf("install binary: %w", err)
	}
	return nil
}

func resolveInstallPath(override string) (string, error) {
	if path := strings.TrimSpace(override); path != "" {
		return path, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate current binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve current binary: %w", err)
	}
	return exe, nil
}
