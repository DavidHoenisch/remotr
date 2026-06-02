package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadConfigFile reads an absolute configuration file path (TLS certs, keys).
// Relative paths and traversal segments are rejected.
func ReadConfigFile(path string) ([]byte, error) {
	clean, err := validateAbsolute(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(clean) // #nosec G304 -- path validated by validateAbsolute
}

// ReadUnderRoot reads a file relative to root; rejects paths that escape root.
func ReadUnderRoot(root string, elems ...string) ([]byte, error) {
	root = filepath.Clean(root)
	if root == "" {
		return nil, fmt.Errorf("invalid root")
	}
	path := filepath.Join(append([]string{root}, elems...)...)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes root")
	}
	return os.ReadFile(path) // #nosec G304 -- path constrained under root via Rel check
}

func validateAbsolute(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}
	return clean, nil
}
