// Package selectorstate stores the bounded ownership set for resources that
// may clean provider-owned state when users leave an authoritative selector.
package selectorstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxOwners = 4096

type Store struct {
	StateDir string
	Key      string
}

type document struct {
	Version int      `json:"version"`
	Users   []string `json:"users"`
}

func (s Store) Load() (map[string]struct{}, error) {
	owners := map[string]struct{}{}
	if strings.TrimSpace(s.StateDir) == "" {
		return owners, nil
	}
	path, err := s.path()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path) // #nosec G304 -- path is a digest under the configured state directory.
	if os.IsNotExist(err) {
		return owners, nil
	}
	if err != nil {
		return nil, err
	}
	var state document
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("decode selector ownership state: %w", err)
	}
	if state.Version != 1 || len(state.Users) > maxOwners {
		return nil, fmt.Errorf("selector ownership state is invalid")
	}
	for _, username := range state.Users {
		if !validUsername(username) {
			return nil, fmt.Errorf("selector ownership state contains an invalid username")
		}
		owners[username] = struct{}{}
	}
	return owners, nil
}

func (s Store) Save(owners map[string]struct{}) error {
	if strings.TrimSpace(s.StateDir) == "" {
		return nil
	}
	if len(owners) > maxOwners {
		return fmt.Errorf("selector ownership state exceeds %d users", maxOwners)
	}
	path, err := s.path()
	if err != nil {
		return err
	}
	users := make([]string, 0, len(owners))
	for username := range owners {
		if !validUsername(username) {
			return fmt.Errorf("selector ownership state contains an invalid username")
		}
		users = append(users, username)
	}
	sort.Strings(users)
	body, err := json.Marshal(document{Version: 1, Users: users})
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".selector-owners-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (s Store) path() (string, error) {
	if !filepath.IsAbs(s.StateDir) {
		return "", fmt.Errorf("selector state directory must be absolute")
	}
	if strings.TrimSpace(s.Key) == "" || strings.ContainsAny(s.Key, "\x00\r\n") {
		return "", fmt.Errorf("selector state key is required")
	}
	digest := sha256.Sum256([]byte(s.Key))
	return filepath.Join(s.StateDir, "interactive-policy-owners", hex.EncodeToString(digest[:])+".json"), nil
}

func validUsername(username string) bool {
	return username != "" && username == strings.TrimSpace(username) && len(username) <= 128 && !strings.ContainsAny(username, "\x00\r\n/")
}
