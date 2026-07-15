package networkstate_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
)

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
	}}
	store, err := networkstate.New(networkstate.Options{Root: t.TempDir(), Runner: runner, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "network-profile-1", Address: "base/uplink", ArtifactDigest: "sha256:artifact", Attempt: 1,
		Backend: "network-manager", Deadline: now.Add(2 * time.Minute), Checkpoint: checkpoint,
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
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "busctl" {
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
