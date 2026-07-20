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
	"time"
)

const stateName = "reboot-state.json"

// Source identifies a successfully applied resource that requested a reboot.
type Source struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type Phase string

const (
	PhaseAwaitingAcknowledgement Phase = "awaiting-acknowledgement"
	PhaseAttempting              Phase = "attempting"
	PhaseTimedOut                Phase = "timed-out"
	PhaseFailed                  Phase = "failed"
)

// Intent is one durable, generation-tagged coordinated reboot attempt.
type Intent struct {
	Generation        string        `json:"generation"`
	Phase             Phase         `json:"phase"`
	PriorBootID       string        `json:"priorBootId"`
	CurrentBootID     string        `json:"currentBootId,omitempty"`
	PreparedAt        time.Time     `json:"preparedAt"`
	NotBefore         time.Time     `json:"notBefore"`
	Timeout           time.Duration `json:"timeout"`
	Deadline          time.Time     `json:"deadline,omitempty"`
	AttemptedAt       time.Time     `json:"attemptedAt,omitempty"`
	AttemptDeadline   time.Time     `json:"attemptDeadline,omitempty"`
	AttemptGeneration uint64        `json:"attemptGeneration,omitempty"`
	Reason            string        `json:"reason,omitempty"`
}

type Completion struct {
	Generation        string    `json:"generation"`
	BootID            string    `json:"bootId"`
	AttemptGeneration uint64    `json:"attemptGeneration"`
	CompletedAt       time.Time `json:"completedAt"`
}

// Status is durable reboot-required and coordinated-attempt state for an endpoint.
type Status struct {
	Required          bool        `json:"required"`
	Sources           []Source    `json:"sources,omitempty"`
	Intent            *Intent     `json:"intent,omitempty"`
	Completion        *Completion `json:"completion,omitempty"`
	AttemptGeneration uint64      `json:"attemptGeneration,omitempty"`
}

type stateFile struct {
	SchemaVersion     int         `json:"schemaVersion"`
	Required          bool        `json:"required"`
	Sources           []Source    `json:"sources,omitempty"`
	Intent            *Intent     `json:"intent,omitempty"`
	Completion        *Completion `json:"completion,omitempty"`
	AttemptGeneration uint64      `json:"attemptGeneration,omitempty"`
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

// Record merges reboot-required resource sources into durable state. A later
// compliant cycle passes no sources and does not clear an outstanding
// requirement.
func (s *Store) Record(sources []Source) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.loadLocked()
	if err != nil {
		return Status{}, err
	}
	changed := false
	for _, source := range sources {
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

// Snapshot reads the complete durable reboot state without changing it.
func (s *Store) Snapshot() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadLocked()
	return cloneStatus(status), err
}

// Prepare persists an intent before it can be included in an authenticated
// pre-reboot Sync. Preparing the same live generation is idempotent.
func (s *Store) Prepare(intent Intent) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if intent.Generation == "" || len(intent.Generation) > 256 || intent.PriorBootID == "" || len(intent.PriorBootID) > 128 || intent.Timeout <= 0 || intent.PreparedAt.IsZero() || intent.NotBefore.IsZero() {
		return Status{}, errors.New("prepare reboot: generation, prior boot ID, timestamps, and timeout are required")
	}
	if !intent.Deadline.IsZero() && !intent.Deadline.After(intent.NotBefore) {
		return Status{}, errors.New("prepare reboot: deadline must follow not-before")
	}
	if intent.Phase == "" {
		intent.Phase = PhaseAwaitingAcknowledgement
	}
	if intent.Phase != PhaseAwaitingAcknowledgement {
		return Status{}, fmt.Errorf("prepare reboot: invalid phase %q", intent.Phase)
	}
	status, err := s.loadLocked()
	if err != nil {
		return Status{}, err
	}
	if status.Completion != nil && status.Completion.Generation == intent.Generation {
		return Status{}, fmt.Errorf("prepare reboot: generation %q is already completed", intent.Generation)
	}
	if status.Intent != nil {
		if status.Intent.Generation == intent.Generation {
			if status.Intent.Phase == PhaseAwaitingAcknowledgement || status.Intent.Phase == PhaseAttempting {
				return cloneStatus(status), nil
			}
			return Status{}, fmt.Errorf("prepare reboot: generation %q is terminal in phase %q", intent.Generation, status.Intent.Phase)
		}
		if status.Intent.Phase == PhaseAwaitingAcknowledgement || status.Intent.Phase == PhaseAttempting {
			return Status{}, fmt.Errorf("prepare reboot: generation %q is already active", status.Intent.Generation)
		}
	}
	intent.Reason = ""
	status.Intent = &intent
	if err := s.saveLocked(status); err != nil {
		return Status{}, err
	}
	return cloneStatus(status), nil
}

// Acknowledge records the authenticated pre-reboot acknowledgement and a new
// durable attempt generation before any reboot command may execute.
func (s *Store) Acknowledge(generation string, now time.Time, currentBootID string) (Intent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadLocked()
	if err != nil {
		return Intent{}, err
	}
	if status.Intent == nil || status.Intent.Generation != generation {
		return Intent{}, fmt.Errorf("acknowledge reboot: generation %q is not prepared", generation)
	}
	intent := *status.Intent
	if intent.Phase != PhaseAwaitingAcknowledgement {
		return Intent{}, fmt.Errorf("acknowledge reboot: generation %q is in phase %q", generation, intent.Phase)
	}
	now = now.UTC()
	if now.Before(intent.NotBefore) {
		return Intent{}, fmt.Errorf("acknowledge reboot: generation %q is delayed until %s", generation, intent.NotBefore.UTC().Format(time.RFC3339))
	}
	if !intent.Deadline.IsZero() && !now.Before(intent.Deadline) {
		intent.Phase = PhaseTimedOut
		intent.Reason = "reboot_deadline_elapsed"
		status.Intent = &intent
		if err := s.saveLocked(status); err != nil {
			return Intent{}, err
		}
		return Intent{}, fmt.Errorf("acknowledge reboot: generation %q deadline elapsed", generation)
	}
	if currentBootID == "" || currentBootID != intent.PriorBootID {
		return Intent{}, fmt.Errorf("acknowledge reboot: boot identity changed before attempt")
	}
	status.AttemptGeneration++
	intent.Phase = PhaseAttempting
	intent.AttemptGeneration = status.AttemptGeneration
	intent.AttemptedAt = now
	intent.AttemptDeadline = now.Add(intent.Timeout)
	if !intent.Deadline.IsZero() && intent.Deadline.Before(intent.AttemptDeadline) {
		intent.AttemptDeadline = intent.Deadline
	}
	intent.Reason = ""
	status.Intent = &intent
	if err := s.saveLocked(status); err != nil {
		return Intent{}, err
	}
	return intent, nil
}

// Reconcile verifies completion using monotonic boot identity. The same boot
// ID never completes an attempt and a timed-out generation cannot self-repeat.
func (s *Store) Reconcile(currentBootID string, now time.Time) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadLocked()
	if err != nil {
		return Status{}, err
	}
	if status.Intent == nil {
		return cloneStatus(status), nil
	}
	now = now.UTC()
	if status.Intent.Phase == PhaseAwaitingAcknowledgement {
		if status.Intent.Deadline.IsZero() || now.Before(status.Intent.Deadline) {
			return cloneStatus(status), nil
		}
		intent := *status.Intent
		intent.Phase = PhaseTimedOut
		intent.Reason = "reboot_deadline_elapsed"
		status.Intent = &intent
		if err := s.saveLocked(status); err != nil {
			return Status{}, err
		}
		return cloneStatus(status), nil
	}
	if status.Intent.Phase != PhaseAttempting {
		return cloneStatus(status), nil
	}
	if currentBootID == "" {
		return Status{}, errors.New("reconcile reboot: current boot ID is required")
	}
	intent := *status.Intent
	intent.CurrentBootID = currentBootID
	if !intent.AttemptDeadline.IsZero() && !now.Before(intent.AttemptDeadline) {
		intent.Phase = PhaseTimedOut
		if currentBootID == intent.PriorBootID {
			intent.Reason = "reboot_timeout_same_boot_id"
		} else {
			intent.Reason = "reboot_timeout"
		}
		status.Intent = &intent
	} else if currentBootID != intent.PriorBootID {
		status.Completion = &Completion{
			Generation: intent.Generation, BootID: currentBootID,
			AttemptGeneration: intent.AttemptGeneration, CompletedAt: now,
		}
		status.Intent = nil
		status.Required = false
		status.Sources = nil
	} else {
		intent.Reason = "boot_id_unchanged"
		status.Intent = &intent
	}
	if err := s.saveLocked(status); err != nil {
		return Status{}, err
	}
	return cloneStatus(status), nil
}

// MarkAttemptFailed records a bounded stable reason after the reboot command
// itself fails. Raw command output must not be passed here.
func (s *Store) MarkAttemptFailed(generation, reason string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadLocked()
	if err != nil {
		return Status{}, err
	}
	if status.Intent == nil || status.Intent.Generation != generation || status.Intent.Phase != PhaseAttempting {
		return Status{}, fmt.Errorf("fail reboot: generation %q is not attempting", generation)
	}
	intent := *status.Intent
	intent.Phase = PhaseFailed
	intent.Reason = reason
	status.Intent = &intent
	if err := s.saveLocked(status); err != nil {
		return Status{}, err
	}
	return cloneStatus(status), nil
}

func (s *Store) Completed(generation, currentBootID string) bool {
	status, err := s.Snapshot()
	return err == nil && status.Completion != nil && status.Completion.Generation == generation && status.Completion.BootID == currentBootID
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
	if persisted.SchemaVersion != 1 && persisted.SchemaVersion != 2 {
		return Status{}, fmt.Errorf("parse reboot state: unsupported schema version %d", persisted.SchemaVersion)
	}
	if persisted.Required != (len(persisted.Sources) > 0) {
		return Status{}, errors.New("parse reboot state: required flag and sources disagree")
	}
	if err := validatePersistedState(persisted); err != nil {
		return Status{}, fmt.Errorf("parse reboot state: %w", err)
	}
	status := Status{
		Required: persisted.Required, Sources: append([]Source(nil), persisted.Sources...),
		Intent: persisted.Intent, Completion: persisted.Completion, AttemptGeneration: persisted.AttemptGeneration,
	}
	return cloneStatus(status), nil
}

func validatePersistedState(persisted stateFile) error {
	if persisted.SchemaVersion == 1 {
		if persisted.Intent != nil || persisted.Completion != nil || persisted.AttemptGeneration != 0 {
			return errors.New("schema version 1 contains coordination state")
		}
		return nil
	}
	if persisted.Intent != nil {
		intent := persisted.Intent
		if intent.Generation == "" || len(intent.Generation) > 256 || intent.PriorBootID == "" || len(intent.PriorBootID) > 128 || intent.PreparedAt.IsZero() || intent.NotBefore.IsZero() || intent.Timeout <= 0 {
			return errors.New("intent is missing required identity, timestamps, or timeout")
		}
		if !intent.Deadline.IsZero() && !intent.Deadline.After(intent.NotBefore) {
			return errors.New("intent deadline must follow not-before")
		}
		if intent.CurrentBootID != "" && intent.CurrentBootID != intent.PriorBootID && intent.Phase != PhaseTimedOut {
			return errors.New("active intent contains a changed boot identity")
		}
		if intent.AttemptGeneration > persisted.AttemptGeneration {
			return errors.New("intent attempt exceeds durable attempt generation")
		}
		switch intent.Phase {
		case PhaseAwaitingAcknowledgement:
			if intent.AttemptGeneration != 0 || !intent.AttemptedAt.IsZero() || !intent.AttemptDeadline.IsZero() || intent.Reason != "" {
				return errors.New("awaiting intent contains attempt state")
			}
		case PhaseAttempting:
			if intent.AttemptGeneration == 0 || intent.AttemptedAt.IsZero() || !intent.AttemptDeadline.After(intent.AttemptedAt) {
				return errors.New("attempting intent is missing attempt state")
			}
		case PhaseTimedOut:
			if intent.Reason == "" {
				return errors.New("timed-out intent is missing a reason")
			}
			if intent.AttemptGeneration > 0 && (intent.AttemptedAt.IsZero() || !intent.AttemptDeadline.After(intent.AttemptedAt)) {
				return errors.New("timed-out intent has invalid attempt state")
			}
		case PhaseFailed:
			if intent.Reason == "" || intent.AttemptGeneration == 0 || intent.AttemptedAt.IsZero() || !intent.AttemptDeadline.After(intent.AttemptedAt) {
				return errors.New("failed intent has invalid attempt state")
			}
		default:
			return fmt.Errorf("unknown intent phase %q", intent.Phase)
		}
	}
	if persisted.Completion != nil {
		completion := persisted.Completion
		if completion.Generation == "" || len(completion.Generation) > 256 || completion.BootID == "" || len(completion.BootID) > 128 || completion.AttemptGeneration == 0 || completion.CompletedAt.IsZero() {
			return errors.New("completion is missing required evidence")
		}
		if completion.AttemptGeneration > persisted.AttemptGeneration {
			return errors.New("completion attempt exceeds durable attempt generation")
		}
		if persisted.Intent != nil && persisted.Intent.Generation == completion.Generation {
			return errors.New("generation cannot be active and completed")
		}
	}
	return nil
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
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect reboot state directory: %w", err)
	}
	raw, err := json.Marshal(stateFile{
		SchemaVersion: 2, Required: status.Required, Sources: status.Sources,
		Intent: status.Intent, Completion: status.Completion, AttemptGeneration: status.AttemptGeneration,
	})
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
	if status.Intent != nil {
		intent := *status.Intent
		status.Intent = &intent
	}
	if status.Completion != nil {
		completion := *status.Completion
		status.Completion = &completion
	}
	return status
}
