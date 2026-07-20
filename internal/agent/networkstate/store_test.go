package networkstate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func TestStoreArmsNetworkManagerRecoveryHandleAcrossRestart(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	root := t.TempDir()
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/41"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointRollback o " + checkpoint + "]": {},
	}}
	options := networkstate.Options{Root: root, Runner: runner, Now: func() time.Time { return now }}
	store, err := networkstate.New(options)
	if err != nil {
		t.Fatal(err)
	}
	intent := networkstate.Intent{
		ID: "network-profile-restart", Address: "networkProfile/uplink",
		ArtifactDigest: "sha256:restart", Attempt: 1, Backend: "network-manager",
		Deadline: now.Add(2 * time.Minute), Checkpoint: checkpoint,
	}
	if _, err := store.Prepare(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	envelope := findTransactionEnvelope(t, root)
	protected, err := os.ReadFile(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(protected, []byte(checkpoint)) {
		t.Fatalf("transaction envelope exposed checkpoint recovery payload: %s", protected)
	}

	// Simulate a process restart followed by local state tampering. Rollback
	// must use the protected handle, not the redirected plaintext checkpoint.
	store, err = networkstate.New(options)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "network-transactions", "state.json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var status networkstate.Status
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatal(err)
	}
	status.Intent.Checkpoint = "/org/freedesktop/NetworkManager/Checkpoint/999"
	raw, err = json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack || status.Intent.Checkpoint != checkpoint {
		t.Fatalf("restart rollback status = %+v", status)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Args[len(runner.Calls[0].Args)-1] != checkpoint {
		t.Fatalf("restart rollback calls = %+v", runner.Calls)
	}

	protected, err = os.ReadFile(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var terminal struct {
		Header struct {
			Metadata struct {
				PayloadPresent bool `json:"payload_present"`
			} `json:"metadata"`
		} `json:"header"`
	}
	if err := json.Unmarshal(protected, &terminal); err != nil || terminal.Header.Metadata.PayloadPresent {
		t.Fatalf("completed recovery payload was not cleaned: present=%t err=%v", terminal.Header.Metadata.PayloadPresent, err)
	}
	store, err = networkstate.New(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reconcile(context.Background()); err != nil || len(runner.Calls) != 1 {
		t.Fatalf("terminal restart repeated rollback: calls=%+v err=%v", runner.Calls, err)
	}
}

func TestStoreRefusesNetworkTransactionWhenRecoveryReservationUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store, err := networkstate.New(networkstate.Options{
		Root: root, Now: func() time.Time { return now },
		RollbackOptions: rollbackstore.Options{
			FilesystemAllowance: 1,
			AvailableBytes:      func(string) (int64, error) { return 0, nil },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "network-profile-no-capacity", Address: "networkProfile/uplink",
		ArtifactDigest: "sha256:no-capacity", Attempt: 1, Backend: "network-manager",
		Deadline: now.Add(2 * time.Minute), Checkpoint: "/org/freedesktop/NetworkManager/Checkpoint/42",
	})
	if !errors.Is(err, rollbackstore.ErrCapacity) || status.Intent != nil {
		t.Fatalf("Prepare() = %+v, %v, want capacity refusal", status, err)
	}
	if got := transactionEnvelopeCount(t, root); got != 0 {
		t.Fatalf("transaction envelope count = %d, want 0 after reservation refusal", got)
	}
	if _, err := os.Stat(filepath.Join(root, "network-transactions", "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists after reservation refusal: %v", err)
	}
}

func TestStorePreflightProbesNetworkReservationWithoutArming(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	intent := networkstate.Intent{
		ID: "network-preflight", Address: "firewall/management",
		ArtifactDigest: "sha256:preflight", Attempt: 1, Backend: "nftables",
		Deadline: now.Add(2 * time.Minute), Snapshot: []byte("flush ruleset\n"),
	}
	for _, tt := range []struct {
		name      string
		available int64
		wantErr   error
	}{
		{name: "capacity blocked", available: 0, wantErr: rollbackstore.ErrCapacity},
		{name: "ready", available: 1 << 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := networkstate.New(networkstate.Options{
				Root: root, Now: func() time.Time { return now },
				RollbackOptions: rollbackstore.Options{
					FilesystemAllowance: 1,
					AvailableBytes:      func(string) (int64, error) { return tt.available, nil },
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = store.Preflight(t.Context(), intent)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Preflight() error = %v, want %v", err, tt.wantErr)
			}
			if got := transactionEnvelopeCount(t, root); got != 0 {
				t.Fatalf("transaction envelope count = %d, want 0", got)
			}
			if status, err := store.Status(); err != nil || status.Intent != nil {
				t.Fatalf("preflight status = %+v, %v", status, err)
			}
		})
	}
}

func TestStoreBlocksOrphanedArmedRecoveryAfterStateLoss(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	root := t.TempDir()
	options := networkstate.Options{Root: root, Now: func() time.Time { return now }}
	store, err := networkstate.New(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "orphaned-firewall", Address: "firewall/guard", ArtifactDigest: "sha256:orphaned",
		Attempt: 1, Backend: "nftables", Deadline: now.Add(2 * time.Minute),
		Snapshot: []byte("flush ruleset\ntable inet filter {}\n"),
	}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "network-transactions", "state.json")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	restarted, err := networkstate.New(options)
	if restarted != nil || !errors.Is(err, rollbackstore.ErrRecoveryBlocked) {
		t.Fatalf("restart with orphaned recovery = %v, %v, want blocking error", restarted, err)
	}
	if got := transactionEnvelopeCount(t, root); got != 1 {
		t.Fatalf("orphaned recovery was removed: envelope count=%d", got)
	}
}

func TestStoreMigratesLegacyNetworkManagerIntentToProtectedHandle(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	root := t.TempDir()
	transactionRoot := filepath.Join(root, "network-transactions")
	if err := os.MkdirAll(transactionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/43"
	legacy := networkstate.Status{Intent: &networkstate.Intent{
		ID: "legacy-network-manager", Address: "networkProfile/legacy",
		ArtifactDigest: "sha256:legacy", Attempt: 1, Backend: "network-manager",
		PreparedAt: now, Deadline: now.Add(2 * time.Minute), Phase: networkstate.PhaseAwaitingAcknowledgement,
		WatchdogArmed: true, Checkpoint: checkpoint,
	}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transactionRoot, "state.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointRollback o " + checkpoint + "]": {},
	}}
	options := networkstate.Options{Root: root, Runner: runner, Now: func() time.Time { return now }}
	store, err := networkstate.New(options)
	if err != nil {
		t.Fatal(err)
	}
	if got := transactionEnvelopeCount(t, root); got != 1 {
		t.Fatalf("migrated transaction envelope count = %d, want 1", got)
	}
	now = now.Add(2 * time.Minute)
	status, err := store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("migrated timeout rollback = %+v, %v", status, err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Args[len(runner.Calls[0].Args)-1] != checkpoint {
		t.Fatalf("migrated checkpoint rollback calls = %+v", runner.Calls)
	}
}

func findTransactionEnvelope(t *testing.T, root string) string {
	t.Helper()
	var found string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || info.Name() != "transaction.envelope" {
			return walkErr
		}
		if found != "" {
			t.Fatalf("multiple transaction envelopes: %s and %s", found, path)
		}
		found = path
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("transaction envelope not found")
	}
	return found
}

func transactionEnvelopeCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	if err := filepath.Walk(root, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		if info.Name() == "transaction.envelope" {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestStoreRollsBackUnacknowledgedNftablesTransactionAtDeadline(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	snapshot := []byte("flush ruleset\ntable inet filter { chain input { tcp dport 8443 accept } }\n")
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [-f -]": {},
	}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	intent := networkstate.Intent{
		ID: "firewall-1", Address: "base/firewall", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "nftables", Deadline: now.Add(2 * time.Minute), Snapshot: snapshot,
	}
	if _, err := store.Prepare(context.Background(), intent); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	status, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack || status.Intent.RollbackReason != "acknowledgement_timeout" {
		t.Fatalf("rollback status = %+v", status)
	}
	if len(runner.Inputs) != 1 || runner.Inputs[0].Name != "nft" || !bytes.Equal(runner.Inputs[0].Input, snapshot) {
		t.Fatalf("snapshot was not restored through protected stdin: %+v", runner.Inputs)
	}
}

func TestStoreAuthenticatedAcknowledgementDisarmsRollback(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"nft [-f -]": {}}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "firewall-1", Address: "base/firewall", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "nftables", Deadline: now.Add(time.Minute), Snapshot: []byte("table inet filter {}\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge(context.Background(), "other"); err == nil {
		t.Fatal("mismatched acknowledgement disarmed rollback")
	}
	status, err := store.Acknowledge(context.Background(), "firewall-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseAcknowledged || status.Intent.WatchdogArmed || !status.Intent.AuthenticatedAck {
		t.Fatalf("acknowledged status = %+v", status)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.Inputs) != 0 {
		t.Fatalf("acknowledged transaction rolled back: %+v", runner.Inputs)
	}
}

func TestStoreRollsBackExpiredNetworkManagerCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/7"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointRollback o " + checkpoint + "]": {},
		"nmcli [-w 30 connection up office ifname eth0]": {},
	}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "network-profile-1", Address: "base/uplink", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "network-manager", Deadline: now.Add(2 * time.Minute), Checkpoint: checkpoint,
		Interface: "eth0", Connection: "office",
	}); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	status, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack || status.Intent.RollbackReason != "acknowledgement_timeout" {
		t.Fatalf("rollback status = %+v", status)
	}
	if len(runner.Calls) != 2 || runner.Calls[0].Name != "busctl" || runner.Calls[1].Name != "nmcli" {
		t.Fatalf("checkpoint rollback calls = %+v", runner.Calls)
	}
}

func TestStoreAuthenticatedAcknowledgementDestroysNetworkManagerCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/8"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointDestroy o " + checkpoint + "]": {},
	}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "network-profile-2", Address: "base/uplink", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "network-manager", Deadline: now.Add(2 * time.Minute), Checkpoint: checkpoint,
	}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Acknowledge(context.Background(), "network-profile-2")
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseAcknowledged || status.Intent.WatchdogArmed || !status.Intent.AuthenticatedAck {
		t.Fatalf("acknowledged status = %+v", status)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Args[4] != "CheckpointDestroy" {
		t.Fatalf("checkpoint destroy calls = %+v", runner.Calls)
	}
}

func TestStoreRestoresSystemdNetworkdFileBeforeReconfiguration(t *testing.T) {
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "90-remotr-uplink.network")
	previous := []byte("[Match]\nName=eth0\n[Network]\nDHCP=yes\n")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"networkctl [reload]":           {},
		"networkctl [reconfigure eth0]": {},
	}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "networkd-1", Address: "base/uplink", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "systemd-networkd", Deadline: now.Add(2 * time.Minute), Snapshot: previous,
		RestorePath: path, RestoreExisted: true, RestoreMode: 0o640, Interface: "eth0",
	}); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	status, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, previous) {
		t.Fatalf("restored network file = %q", restored)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode = %v, err=%v", info.Mode().Perm(), err)
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack || len(runner.Calls) != 2 {
		t.Fatalf("rollback status = %+v, calls=%+v", status, runner.Calls)
	}
}

func TestStoreAcknowledgementDisarmsFileBackedNetworkRollback(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "90-remotr-uplink.yaml")
	changed := []byte("changed configuration\n")
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "netplan-1", Address: "base/uplink", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "netplan", Deadline: now.Add(2 * time.Minute), Snapshot: []byte("previous\n"),
		RestorePath: path, RestoreExisted: true, RestoreMode: 0o600, Interface: "eth0",
	}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Acknowledge(context.Background(), "netplan-1")
	if err != nil || status.Intent == nil || !status.Intent.AuthenticatedAck || status.Intent.WatchdogArmed {
		t.Fatalf("acknowledgement status = %+v, err=%v", status, err)
	}
	now = now.Add(3 * time.Minute)
	if _, err := store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, changed) || len(runner.Calls) != 0 {
		t.Fatalf("acknowledged configuration = %q, calls=%+v, err=%v", actual, runner.Calls, err)
	}
}
