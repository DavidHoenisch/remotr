package interactiveuser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MinUID is the minimum UID for interactive (human login) accounts from passwd.
const MinUID = 1000

// Account is a local passwd entry eligible for per-user resources.
type Account struct {
	Username string
	UID      int
	GID      int
	HomeDir  string
}

var passwdPath = "/etc/passwd"

// List reads interactive accounts from /etc/passwd (UID >= MinUID, excluding nobody).
func List() ([]Account, error) {
	data, err := os.ReadFile(passwdPath) // #nosec G304 -- fixed system path
	if err != nil {
		return nil, err
	}
	return ParsePasswd(string(data))
}

// ParsePasswd parses passwd content without reading the filesystem.
func ParsePasswd(content string) ([]Account, error) {
	var users []Account
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, ok, err := parsePasswdLine(line)
		if err != nil {
			return nil, err
		}
		if ok {
			users = append(users, u)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func parsePasswdLine(line string) (Account, bool, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 6 {
		return Account{}, false, fmt.Errorf("invalid passwd line: %q", line)
	}
	if len(fields) < 7 {
		return Account{}, false, nil
	}
	username := fields[0]
	uid, err := strconv.Atoi(fields[2])
	if err != nil {
		return Account{}, false, fmt.Errorf("invalid uid in passwd line: %q", line)
	}
	gid, err := strconv.Atoi(fields[3])
	if err != nil {
		return Account{}, false, fmt.Errorf("invalid gid in passwd line: %q", line)
	}
	if uid < MinUID || username == "nobody" {
		return Account{}, false, nil
	}
	home := fields[5]
	if home == "" {
		return Account{}, false, fmt.Errorf("missing home directory in passwd line: %q", line)
	}
	if !filepath.IsAbs(home) {
		return Account{}, false, fmt.Errorf("home directory must be absolute in passwd line: %q", line)
	}
	if home == "/" {
		return Account{}, false, nil
	}
	shell := fields[6]
	if isNonInteractiveShell(shell) {
		return Account{}, false, nil
	}
	return Account{
		Username: username,
		UID:      uid,
		GID:      gid,
		HomeDir:  filepath.Clean(home),
	}, true, nil
}

// HomePath joins a user's home directory with a relative path (no .. segments).
func HomePath(home, relative string) (string, error) {
	rel := strings.TrimSpace(relative)
	if rel == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to the user home directory")
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}
	return filepath.Join(home, clean), nil
}

func isNonInteractiveShell(shell string) bool {
	switch strings.TrimSpace(shell) {
	case "", "/usr/sbin/nologin", "/sbin/nologin", "/bin/false", "/usr/bin/false":
		return true
	default:
		return strings.HasSuffix(shell, "/nologin") || strings.HasSuffix(shell, "/false")
	}
}
