// Package networkstate persists guarded network/firewall transaction state so
// acknowledgement timeout recovery survives agent restarts.
package networkstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

var ErrAwaitingAcknowledgement = errors.New("network transaction is awaiting acknowledgement")

type Phase string

const (
	PhaseAwaitingAcknowledgement Phase = "awaiting-acknowledgement"
	PhaseAcknowledged            Phase = "acknowledged"
	PhaseRolledBack              Phase = "rolled-back"
	PhaseRollbackFailed          Phase = "rollback-failed"
)

type Intent struct {
	ID               string    `json:"id"`
	Address          string    `json:"address"`
	ArtifactDigest   string    `json:"artifactDigest"`
	Attempt          int       `json:"attempt"`
	Backend          string    `json:"backend"`
	PreparedAt       time.Time `json:"preparedAt"`
	Deadline         time.Time `json:"deadline"`
	Phase            Phase     `json:"phase"`
	RollbackReason   string    `json:"rollbackReason,omitempty"`
	RollbackError    string    `json:"rollbackError,omitempty"`
	PlanHash         string    `json:"planHash,omitempty"`
	WatchdogArmed    bool      `json:"watchdogArmed"`
	AuthenticatedAck bool      `json:"authenticatedAck,omitempty"`
	Checkpoint       string    `json:"checkpoint,omitempty"`
	Snapshot         []byte    `json:"-"`
}

type Status struct {
	Intent *Intent `json:"intent,omitempty"`
}

type Options struct {
	Root   string
	Runner executil.Runner
	Now    func() time.Time
}

type Store struct {
	root     string
	runner   executil.Runner
	now      func() time.Time
	rollback *rollbackstore.Store
}

func New(options Options) (*Store, error) {
	if options.Root == "" {
		return nil, errors.New("network transaction root is required")
	}
	if options.Runner == nil {
		options.Runner = executil.OSRunner{}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	root := filepath.Join(options.Root, "network-transactions")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	rollback, err := rollbackstore.New(rollbackstore.Options{Root: filepath.Join(root, "rollback"), Now: options.Now})
	if err != nil {
		return nil, err
	}
	return &Store{root: root, runner: options.Runner, now: options.Now, rollback: rollback}, nil
}

func (s *Store) Prepare(ctx context.Context, intent Intent) (Status, error) {
	if intent.ID == "" || intent.Address == "" || intent.ArtifactDigest == "" || intent.Attempt < 1 {
		return Status{}, errors.New("network transaction requires id, address, artifact digest, and positive attempt")
	}
	if intent.Backend != "nftables" && intent.Backend != "network-manager" {
		return Status{}, fmt.Errorf("network transaction backend %q has no transactional restore", intent.Backend)
	}
	now := s.now().UTC()
	if !intent.Deadline.After(now) {
		return Status{}, errors.New("network transaction deadline must be in the future")
	}
	if intent.Backend == "nftables" && len(intent.Snapshot) == 0 {
		return Status{}, errors.New("network transaction snapshot is required")
	}
	if intent.Backend == "network-manager" && !strings.HasPrefix(intent.Checkpoint, "/org/freedesktop/NetworkManager/Checkpoint/") {
		return Status{}, errors.New("network-manager transaction checkpoint is required")
	}
	if current, err := s.Status(); err != nil {
		return Status{}, err
	} else if current.Intent != nil && current.Intent.Phase == PhaseAwaitingAcknowledgement {
		return Status{}, fmt.Errorf("%w: %s", ErrAwaitingAcknowledgement, current.Intent.ID)
	}
	if intent.Backend == "nftables" {
		if err := s.rollback.Save(ctx, rollbackstore.Record{
			Address: intent.Address, ArtifactDigest: intent.ArtifactDigest, Attempt: intent.Attempt,
			Payload: intent.Snapshot, Armed: true,
		}); err != nil {
			return Status{}, fmt.Errorf("reserve network rollback: %w", err)
		}
	}
	intent.Snapshot = nil
	intent.PreparedAt = now
	intent.Phase = PhaseAwaitingAcknowledgement
	intent.WatchdogArmed = true
	status := Status{Intent: &intent}
	if err := s.write(status); err != nil {
		if intent.Backend == "nftables" {
			_ = s.rollback.Delete(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt)
		}
		return Status{}, err
	}
	return status, nil
}

// Rollback immediately restores an armed transaction, retaining the caller's
// safe reason in durable state.
func (s *Store) Rollback(ctx context.Context, reason string) (Status, error) {
	status, err := s.Status()
	if err != nil {
		return status, err
	}
	if status.Intent == nil || status.Intent.Phase != PhaseAwaitingAcknowledgement {
		return status, fmt.Errorf("no armed network transaction to roll back")
	}
	if reason == "" {
		reason = "requested"
	}
	return s.rollbackIntent(ctx, *status.Intent, reason)
}

func (s *Store) Status() (Status, error) {
	raw, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return Status{}, fmt.Errorf("decode network transaction state: %w", err)
	}
	return status, nil
}

func (s *Store) Reconcile(ctx context.Context) (Status, error) {
	status, err := s.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != PhaseAwaitingAcknowledgement {
		return status, err
	}
	if s.now().UTC().Before(status.Intent.Deadline) {
		return status, nil
	}
	return s.rollbackIntent(ctx, *status.Intent, "acknowledgement_timeout")
}

func (s *Store) Acknowledge(ctx context.Context, id string) (Status, error) {
	status, err := s.Reconcile(ctx)
	if err != nil {
		return status, err
	}
	if status.Intent == nil || status.Intent.ID != id {
		return status, fmt.Errorf("network transaction %q is not prepared", id)
	}
	if status.Intent.Phase != PhaseAwaitingAcknowledgement {
		return status, fmt.Errorf("network transaction %q is in phase %q", id, status.Intent.Phase)
	}
	intent := *status.Intent
	if intent.Backend == "network-manager" {
		if _, _, err := s.runner.Run("busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager", "org.freedesktop.NetworkManager", "CheckpointDestroy", "o", intent.Checkpoint); err != nil {
			return status, fmt.Errorf("destroy acknowledged NetworkManager checkpoint: %w", err)
		}
	}
	intent.Phase = PhaseAcknowledged
	intent.WatchdogArmed = false
	intent.AuthenticatedAck = true
	status.Intent = &intent
	if err := s.write(status); err != nil {
		return Status{}, err
	}
	if intent.Backend == "nftables" {
		if err := s.rollback.Delete(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt); err != nil {
			return status, err
		}
	}
	return status, nil
}

func (s *Store) rollbackIntent(ctx context.Context, intent Intent, reason string) (Status, error) {
	if intent.Backend == "network-manager" {
		_, _, err := s.runner.Run("busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager", "org.freedesktop.NetworkManager", "CheckpointRollback", "o", intent.Checkpoint)
		if err != nil {
			return s.markRollbackFailure(intent, reason, fmt.Errorf("restore NetworkManager checkpoint: %w", err))
		}
		intent.Phase = PhaseRolledBack
		intent.RollbackReason = reason
		intent.WatchdogArmed = false
		status := Status{Intent: &intent}
		if err := s.write(status); err != nil {
			return Status{}, err
		}
		return status, nil
	}
	payload, err := s.rollback.Load(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt)
	if err != nil {
		return s.markRollbackFailure(intent, reason, fmt.Errorf("load protected snapshot: %w", err))
	}
	input, ok := s.runner.(executil.InputRunner)
	if !ok {
		return s.markRollbackFailure(intent, reason, errors.New("runner does not support protected rollback input"))
	}
	_, _, restoreErr := input.RunInput("nft", payload, "-f", "-")
	if restoreErr != nil {
		return s.markRollbackFailure(intent, reason, fmt.Errorf("restore nftables snapshot: %w", restoreErr))
	}
	intent.Phase = PhaseRolledBack
	intent.RollbackReason = reason
	intent.WatchdogArmed = false
	status := Status{Intent: &intent}
	if err := s.write(status); err != nil {
		return Status{}, err
	}
	if err := s.rollback.Delete(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt); err != nil {
		return status, err
	}
	return status, nil
}

func (s *Store) markRollbackFailure(intent Intent, reason string, rollbackErr error) (Status, error) {
	intent.Phase = PhaseRollbackFailed
	intent.RollbackReason = reason
	intent.RollbackError = rollbackErr.Error()
	intent.WatchdogArmed = false
	status := Status{Intent: &intent}
	if err := s.write(status); err != nil {
		return Status{}, errors.Join(rollbackErr, err)
	}
	return status, rollbackErr
}

func (s *Store) path() string { return filepath.Join(s.root, "state.json") }

func (s *Store) write(status Status) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path())
}
