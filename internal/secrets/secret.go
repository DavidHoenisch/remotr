package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileRef reads a secret from file:/absolute/path (trailing newlines stripped).
func ReadFileRef(ref string) (string, error) {
	s := strings.TrimSpace(ref)
	if !strings.HasPrefix(s, "file:") {
		return "", fmt.Errorf("secret ref must be file:/absolute/path")
	}
	path := strings.TrimSpace(s[len("file:"):])
	if path == "" {
		return "", fmt.Errorf("secret ref file path is empty")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("secret ref path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid secret ref path")
	}
	data, err := os.ReadFile(clean) // #nosec G304 -- absolute path validated
	if err != nil {
		return "", fmt.Errorf("read secret %s: %w", clean, err)
	}
	out := strings.TrimSpace(string(data))
	if out == "" {
		return "", fmt.Errorf("secret file %s is empty", clean)
	}
	return out, nil
}
