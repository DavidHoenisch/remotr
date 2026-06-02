package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	certName = "agent.crt"
	keyName  = "agent.key"
	caName   = "ca.crt"
	metaName = "state.json"
)

// DirLayout holds absolute paths to persisted endpoint credentials.
type DirLayout struct {
	Dir  string
	Cert string
	Key  string
	CA   string
	Meta string
}

// State records enrollment metadata written alongside TLS material.
type State struct {
	EndpointID string `json:"endpoint_id"`
}

// Present reports whether a complete credential set exists under dir.
func Present(dir string) bool {
	p, err := Layout(dir)
	if err != nil {
		return false
	}
	for _, path := range []string{p.Cert, p.Key, p.CA, p.Meta} {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

// Layout returns absolute file paths for credentials stored under dir.
func Layout(dir string) (DirLayout, error) {
	dir = filepath.Clean(dir)
	if dir == "" {
		return DirLayout{}, errors.New("credentials directory is required")
	}
	return DirLayout{
		Dir:  dir,
		Cert: filepath.Join(dir, certName),
		Key:  filepath.Join(dir, keyName),
		CA:   filepath.Join(dir, caName),
		Meta: filepath.Join(dir, metaName),
	}, nil
}

// Save writes endpoint credentials and metadata under dir with restrictive permissions.
func Save(dir, endpointID, certPEM, keyPEM, caPEM string) error {
	if endpointID == "" || certPEM == "" || keyPEM == "" || caPEM == "" {
		return errors.New("incomplete credential material")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir credentials dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302 -- directory permissions
		return fmt.Errorf("chmod credentials dir: %w", err)
	}

	p, err := Layout(dir)
	if err != nil {
		return err
	}

	files := map[string]string{
		p.Cert: certPEM,
		p.Key:  keyPEM,
		p.CA:   caPEM,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	meta, err := json.Marshal(State{EndpointID: endpointID})
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.Meta, meta, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// LoadState reads enrollment metadata from dir.
func LoadState(dir string) (State, error) {
	p, err := Layout(dir)
	if err != nil {
		return State{}, err
	}
	raw, err := os.ReadFile(p.Meta)
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	return st, nil
}
