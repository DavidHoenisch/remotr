// Package rebootstate persists reboot-required evidence independently from
// reboot execution. Recording or reading this state never initiates a reboot.
package rebootstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

const stateName = "reboot-state.json"

// Source identifies a successfully applied resource that requested a reboot.
type Source struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Status is durable reboot-required state for an endpoint.
type Status struct {
	Required bool     `json:"required"`
	Sources  []Source `json:"sources,omitempty"`
}

type stateFile struct {
	SchemaVersion int      `json:"schemaVersion"`
	Required      bool     `json:"required"`
	Sources       []Source `json:"sources,omitempty"`
}

// Store owns the endpoint-local reboot state file. An empty state directory
// retains evidence in memory for compatibility, but cannot survive restart.
type Store struct {
	mu       sync.Mutex
	path     string
	volatile Status
}

// New creates a tracker rooted in the agent state directory.
func New(stateDir string) *Store {
	path := ""
	if stateDir != "" {
		path = filepath.Join(stateDir, stateName)
	}
	return &Store{path: path}
}

// Path returns the durable state path, or an empty string for an in-memory
// tracker.
func (s *Store) Path() string { return s.path }

// Record merges successful reboot-required apply outcomes into durable state.
// A later compliant apply does not clear an outstanding requirement.
func (s *Store) Record(applied engine.ApplyResult) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.loadLocked()
	if err != nil {
		return Status{}, err
	}
	changed := false
	for _, item := range applied.Items {
		if item.RebootRequired != executor.RebootRequired || (item.Status != executor.Changed && item.Status != executor.NoChange) {
			continue
		}
		source := Source{Address: item.Address, Name: item.Name, Provider: item.Provider}
		if !slices.Contains(status.Sources, source) {
			status.Sources = append(status.Sources, source)
			changed = true
		}
	}
	if len(status.Sources) > 0 && !status.Required {
		status.Required = true
		changed = true
	}
	slices.SortFunc(status.Sources, func(a, b Source) int {
		if a.Address != b.Address {
			if a.Address < b.Address {
				return -1
			}
			return 1
		}
		if a.Provider < b.Provider {
			return -1
		}
		if a.Provider > b.Provider {
			return 1
		}
		return 0
	})
	if !changed {
		return cloneStatus(status), nil
	}
	if err := s.saveLocked(status); err != nil {
		return Status{}, err
	}
	return cloneStatus(status), nil
}

func (s *Store) loadLocked() (Status, error) {
	if s.path == "" {
		return cloneStatus(s.volatile), nil
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("read reboot state: %w", err)
	}
	return parseState(raw)
}

func parseState(raw []byte) (Status, error) {
	var persisted stateFile
	if err := json.Unmarshal(raw, &persisted); err != nil {
		return Status{}, fmt.Errorf("parse reboot state: %w", err)
	}
	if persisted.SchemaVersion != 1 {
		return Status{}, fmt.Errorf("parse reboot state: unsupported schema version %d", persisted.SchemaVersion)
	}
	if persisted.Required != (len(persisted.Sources) > 0) {
		return Status{}, errors.New("parse reboot state: required flag and sources disagree")
	}
	return Status{Required: persisted.Required, Sources: append([]Source(nil), persisted.Sources...)}, nil
}

func (s *Store) saveLocked(status Status) error {
	if s.path == "" {
		s.volatile = cloneStatus(status)
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create reboot state directory: %w", err)
	}
	raw, err := json.Marshal(stateFile{SchemaVersion: 1, Required: status.Required, Sources: status.Sources})
	if err != nil {
		return fmt.Errorf("marshal reboot state: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".reboot-state-")
	if err != nil {
		return fmt.Errorf("create reboot state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect reboot state: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return fmt.Errorf("write reboot state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync reboot state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close reboot state: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("activate reboot state: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open reboot state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync reboot state directory: %w", err)
	}
	return nil
}

func cloneStatus(status Status) Status {
	status.Sources = append([]Source(nil), status.Sources...)
	return status
}
