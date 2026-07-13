package networkstate_test

import (
	"bytes"
	"context"
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
