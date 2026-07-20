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
	RestorePath      string    `json:"restorePath,omitempty"`
	RestoreExisted   bool      `json:"restoreExisted,omitempty"`
	RestoreMode      uint32    `json:"restoreMode,omitempty"`
	Interface        string    `json:"interface,omitempty"`
	Connection       string    `json:"connection,omitempty"`
	Snapshot         []byte    `json:"-"`
}

type Status struct {
	Intent *Intent `json:"intent,omitempty"`
}

type Options struct {
	Root            string
	Runner          executil.Runner
	Now             func() time.Time
	RollbackOptions rollbackstore.Options
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
	rollbackOptions := options.RollbackOptions
	rollbackOptions.Root = filepath.Join(root, "rollback")
	rollbackOptions.Now = options.Now
	rollback, err := rollbackstore.New(rollbackOptions)
	if err != nil {
		return nil, err
	}
	store := &Store{root: root, runner: options.Runner, now: options.Now, rollback: rollback}
	if err := store.validateStartup(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

const protectedRecoveryVersion = 1

type protectedRecovery struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	Address        string    `json:"address"`
	ArtifactDigest string    `json:"artifactDigest"`
	Attempt        int       `json:"attempt"`
	Backend        string    `json:"backend"`
	PreparedAt     time.Time `json:"preparedAt"`
	Deadline       time.Time `json:"deadline"`
	PlanHash       string    `json:"planHash,omitempty"`
	Checkpoint     string    `json:"checkpoint,omitempty"`
	RestorePath    string    `json:"restorePath,omitempty"`
	RestoreExisted bool      `json:"restoreExisted,omitempty"`
	RestoreMode    uint32    `json:"restoreMode,omitempty"`
	Interface      string    `json:"interface,omitempty"`
	Connection     string    `json:"connection,omitempty"`
	Snapshot       []byte    `json:"snapshot,omitempty"`
}

func protectedRecoveryFromIntent(intent Intent) protectedRecovery {
	return protectedRecovery{
		Version: protectedRecoveryVersion,
		ID:      intent.ID, Address: intent.Address, ArtifactDigest: intent.ArtifactDigest,
		Attempt: intent.Attempt, Backend: intent.Backend, PreparedAt: intent.PreparedAt,
		Deadline: intent.Deadline, PlanHash: intent.PlanHash, Checkpoint: intent.Checkpoint,
		RestorePath: intent.RestorePath, RestoreExisted: intent.RestoreExisted,
		RestoreMode: intent.RestoreMode, Interface: intent.Interface, Connection: intent.Connection,
		Snapshot: intent.Snapshot,
	}
}

func (recovery protectedRecovery) restore(intent Intent) Intent {
	intent.ID = recovery.ID
	intent.Address = recovery.Address
	intent.ArtifactDigest = recovery.ArtifactDigest
	intent.Attempt = recovery.Attempt
	intent.Backend = recovery.Backend
	intent.PreparedAt = recovery.PreparedAt
	intent.Deadline = recovery.Deadline
	intent.PlanHash = recovery.PlanHash
	intent.Checkpoint = recovery.Checkpoint
	intent.RestorePath = recovery.RestorePath
	intent.RestoreExisted = recovery.RestoreExisted
	intent.RestoreMode = recovery.RestoreMode
	intent.Interface = recovery.Interface
	intent.Connection = recovery.Connection
	intent.Snapshot = nil
	return intent
}

func (s *Store) loadProtectedRecovery(ctx context.Context, intent Intent) (protectedRecovery, error) {
	payload, err := s.rollback.Load(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt)
	if err != nil {
		return protectedRecovery{}, err
	}
	defer clear(payload)
	var recovery protectedRecovery
	if err := json.Unmarshal(payload, &recovery); err != nil {
		// Version zero network transactions stored the raw nftables or file
		// snapshot. Preserve that recovery path during in-place migration.
		if intent.Backend == "network-manager" {
			return protectedRecovery{}, errors.New("protected NetworkManager recovery handle is malformed")
		}
		recovery = protectedRecoveryFromIntent(intent)
		recovery.Snapshot = append([]byte(nil), payload...)
		return recovery, nil
	}
	if recovery.Version != protectedRecoveryVersion || recovery.Address != intent.Address ||
		recovery.ArtifactDigest != intent.ArtifactDigest || recovery.Attempt != intent.Attempt {
		clear(recovery.Snapshot)
		return protectedRecovery{}, errors.New("protected network recovery identity is invalid")
	}
	if recovery.ID == "" || recovery.Backend == "" || !recovery.Deadline.After(recovery.PreparedAt) {
		clear(recovery.Snapshot)
		return protectedRecovery{}, errors.New("protected network recovery handle is incomplete")
	}
	return recovery, nil
}

func (s *Store) validateStartup(ctx context.Context) error {
	status, err := s.Status()
	if err != nil {
		return fmt.Errorf("%w: read network transaction state: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	records, err := s.rollback.Records(ctx, "")
	if err != nil {
		return fmt.Errorf("%w: read protected network transactions: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	armed := make([]rollbackstore.RecordInfo, 0, 1)
	for _, record := range records {
		if record.State == rollbackstore.LifecycleArmed {
			armed = append(armed, record)
		}
	}
	if status.Intent == nil || status.Intent.Phase != PhaseAwaitingAcknowledgement {
		if len(armed) != 0 {
			return fmt.Errorf("%w: %d armed network recoveries have no awaiting transaction state", rollbackstore.ErrRecoveryBlocked, len(armed))
		}
		return nil
	}
	intent := *status.Intent
	matching := 0
	for _, record := range armed {
		if record.Address == intent.Address && record.ArtifactDigest == intent.ArtifactDigest && record.Attempt == intent.Attempt {
			matching++
		}
	}
	if len(armed) == 1 && matching == 1 {
		return nil
	}
	if len(armed) != 0 {
		return fmt.Errorf("%w: awaiting network transaction does not uniquely match its armed recovery", rollbackstore.ErrRecoveryBlocked)
	}
	if intent.Backend != "network-manager" {
		return fmt.Errorf("%w: awaiting network transaction has no armed recovery", rollbackstore.ErrRecoveryBlocked)
	}
	if intent.ID == "" || intent.Address == "" || intent.ArtifactDigest == "" || intent.Attempt < 1 ||
		!strings.HasPrefix(intent.Checkpoint, "/org/freedesktop/NetworkManager/Checkpoint/") ||
		!intent.Deadline.After(intent.PreparedAt) {
		return fmt.Errorf("%w: legacy NetworkManager recovery state is invalid", rollbackstore.ErrRecoveryBlocked)
	}
	protected, err := json.Marshal(protectedRecoveryFromIntent(intent))
	if err != nil {
		return fmt.Errorf("%w: encode legacy NetworkManager recovery: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	defer clear(protected)
	reservation, err := s.rollback.Reserve(ctx, rollbackstore.ReservationRequest{
		Address: intent.Address, ArtifactDigest: intent.ArtifactDigest, Attempt: intent.Attempt,
		PayloadBytes: int64(len(protected)),
	})
	if err != nil {
		return fmt.Errorf("%w: reserve legacy NetworkManager recovery: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	if err := reservation.Arm(ctx, protected); err != nil {
		return fmt.Errorf("%w: arm legacy NetworkManager recovery: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	return nil
}

func (s *Store) Prepare(ctx context.Context, intent Intent) (Status, error) {
	prepared, protected, reservation, err := s.reserveIntent(ctx, intent)
	if err != nil {
		return Status{}, err
	}
	if err := reservation.Arm(ctx, protected); err != nil {
		clear(protected)
		return Status{}, fmt.Errorf("arm network rollback: %w", err)
	}
	clear(protected)
	prepared.Snapshot = nil
	status := Status{Intent: &prepared}
	if err := s.write(status); err != nil {
		_ = s.rollback.Delete(ctx, prepared.Address, prepared.ArtifactDigest, prepared.Attempt)
		return Status{}, err
	}
	return status, nil
}

// Preflight proves the exact protected recovery payload can be reserved while
// leaving no envelope or intent state behind.
func (s *Store) Preflight(ctx context.Context, intent Intent) error {
	_, protected, reservation, err := s.reserveIntent(ctx, intent)
	if err != nil {
		return err
	}
	reservation.Release()
	clear(protected)
	return nil
}

func (s *Store) reserveIntent(ctx context.Context, intent Intent) (Intent, []byte, *rollbackstore.Reservation, error) {
	if intent.ID == "" || intent.Address == "" || intent.ArtifactDigest == "" || intent.Attempt < 1 {
		return Intent{}, nil, nil, errors.New("network transaction requires id, address, artifact digest, and positive attempt")
	}
	fileBackend := intent.Backend == "netplan" || intent.Backend == "systemd-networkd"
	if intent.Backend != "nftables" && intent.Backend != "network-manager" && !fileBackend {
		return Intent{}, nil, nil, fmt.Errorf("network transaction backend %q has no transactional restore", intent.Backend)
	}
	now := s.now().UTC()
	if !intent.Deadline.After(now) {
		return Intent{}, nil, nil, errors.New("network transaction deadline must be in the future")
	}
	if fileBackend && intent.Deadline.After(now.Add(rollbackstore.MaxSensitiveRetention)) {
		return Intent{}, nil, nil, fmt.Errorf("file-backed network rollback deadline exceeds %s", rollbackstore.MaxSensitiveRetention)
	}
	if intent.Backend == "nftables" && len(intent.Snapshot) == 0 {
		return Intent{}, nil, nil, errors.New("network transaction snapshot is required")
	}
	if intent.Backend == "network-manager" && !strings.HasPrefix(intent.Checkpoint, "/org/freedesktop/NetworkManager/Checkpoint/") {
		return Intent{}, nil, nil, errors.New("network-manager transaction checkpoint is required")
	}
	if fileBackend {
		if !filepath.IsAbs(intent.RestorePath) || filepath.Clean(intent.RestorePath) != intent.RestorePath || strings.ContainsAny(intent.Interface, "/\\\x00\r\n") || intent.Interface == "" {
			return Intent{}, nil, nil, errors.New("file-backed network transaction restore target is invalid")
		}
		mode := os.FileMode(intent.RestoreMode)
		if intent.RestoreExisted && (mode.Perm() == 0 || mode.Perm()&0o111 != 0) {
			return Intent{}, nil, nil, errors.New("file-backed network transaction restore mode is invalid")
		}
	}
	if current, err := s.Status(); err != nil {
		return Intent{}, nil, nil, err
	} else if current.Intent != nil && current.Intent.Phase == PhaseAwaitingAcknowledgement {
		return Intent{}, nil, nil, fmt.Errorf("%w: %s", ErrAwaitingAcknowledgement, current.Intent.ID)
	}
	intent.PreparedAt = now
	intent.Phase = PhaseAwaitingAcknowledgement
	intent.WatchdogArmed = true
	protected, err := json.Marshal(protectedRecoveryFromIntent(intent))
	if err != nil {
		return Intent{}, nil, nil, err
	}
	expiresAt := time.Time{}
	if fileBackend {
		expiresAt = now.Add(rollbackstore.MaxSensitiveRetention)
	}
	reservation, err := s.rollback.Reserve(ctx, rollbackstore.ReservationRequest{
		Address: intent.Address, ArtifactDigest: intent.ArtifactDigest, Attempt: intent.Attempt,
		PayloadBytes: int64(len(protected)), Sensitive: fileBackend, ExpiresAt: expiresAt,
	})
	if err != nil {
		clear(protected)
		return Intent{}, nil, nil, fmt.Errorf("reserve network rollback: %w", err)
	}
	return intent, protected, reservation, nil
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
	protected, err := s.loadProtectedRecovery(ctx, *status.Intent)
	if err != nil {
		return status, fmt.Errorf("%w: validate protected network recovery: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	intent := protected.restore(*status.Intent)
	status.Intent = &intent
	clear(protected.Snapshot)
	if s.now().UTC().Before(intent.Deadline) {
		return status, nil
	}
	return s.rollbackIntent(ctx, intent, "acknowledgement_timeout")
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
	if err := s.rollback.Delete(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt); err != nil {
		return status, err
	}
	return status, nil
}

func (s *Store) rollbackIntent(ctx context.Context, intent Intent, reason string) (Status, error) {
	protected, err := s.loadProtectedRecovery(ctx, intent)
	if err != nil {
		return Status{Intent: &intent}, fmt.Errorf("%w: load protected network recovery: %w", rollbackstore.ErrRecoveryBlocked, err)
	}
	defer clear(protected.Snapshot)
	intent = protected.restore(intent)
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
		if err := s.rollback.Delete(ctx, intent.Address, intent.ArtifactDigest, intent.Attempt); err != nil {
			return status, err
		}
		return status, nil
	}
	if intent.Backend == "netplan" || intent.Backend == "systemd-networkd" {
		if intent.RestoreExisted {
			if err := writeRestoreAtomic(intent.RestorePath, protected.Snapshot, os.FileMode(intent.RestoreMode)); err != nil {
				return s.markRollbackFailure(intent, reason, fmt.Errorf("restore network configuration: %w", err))
			}
		} else if err := os.Remove(intent.RestorePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return s.markRollbackFailure(intent, reason, fmt.Errorf("remove new network configuration: %w", err))
		}
		if err := s.activateFileBackend(intent); err != nil {
			return s.markRollbackFailure(intent, reason, err)
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
	input, ok := s.runner.(executil.InputRunner)
	if !ok {
		return s.markRollbackFailure(intent, reason, errors.New("runner does not support protected rollback input"))
	}
	_, _, restoreErr := input.RunInput("nft", protected.Snapshot, "-f", "-")
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

func (s *Store) activateFileBackend(intent Intent) error {
	switch intent.Backend {
	case "netplan":
		if _, _, err := s.runner.Run("netplan", "generate"); err != nil {
			return fmt.Errorf("validate restored netplan configuration: %w", err)
		}
		if _, _, err := s.runner.Run("netplan", "apply"); err != nil {
			return fmt.Errorf("apply restored netplan configuration: %w", err)
		}
	case "systemd-networkd":
		if _, _, err := s.runner.Run("networkctl", "reload"); err != nil {
			return fmt.Errorf("reload restored systemd-networkd configuration: %w", err)
		}
		if _, _, err := s.runner.Run("networkctl", "reconfigure", intent.Interface); err != nil {
			return fmt.Errorf("reconfigure restored systemd-networkd interface: %w", err)
		}
	}
	return nil
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

func writeRestoreAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-network-restore-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode.Perm()); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
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
	return os.Rename(name, path)
}

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
