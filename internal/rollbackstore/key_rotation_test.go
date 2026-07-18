package rollbackstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func TestStoreKeyRotationRetainsHistoricalDecryptOnlyRecovery(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	provider := newRotatingTestKeyProvider()
	store, err := rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	before := rollbackstore.Record{
		Address: "base/before-rotation", ArtifactDigest: "sha256:before", Attempt: 1,
		Payload: []byte("recovery protected by historical key"), Armed: true,
	}
	if err := store.Save(ctx, before); err != nil {
		t.Fatal(err)
	}
	retainedBefore := rollbackstore.Record{
		Address: "base/retained-before-rotation", ArtifactDigest: "sha256:retained-before", Attempt: 1,
		Payload: []byte("successful prior state protected by historical key"), Successful: true,
	}
	if err := store.Save(ctx, retainedBefore); err != nil {
		t.Fatal(err)
	}
	report, err := store.RotateKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.KeyID != provider.rotated.ID || report.Class != rollbackstore.ProtectionRootFile {
		t.Fatalf("rotation report = %+v, want new safe key identity", report)
	}
	if got, err := store.Load(ctx, before.Address, before.ArtifactDigest, before.Attempt); err != nil || !bytes.Equal(got, before.Payload) {
		t.Fatalf("pre-rotation recovery = %q, %v", got, err)
	}
	after := rollbackstore.Record{
		Address: "base/after-rotation", ArtifactDigest: "sha256:after", Attempt: 1,
		Payload: []byte("recovery protected by active key"), Armed: true,
	}
	if err := store.Save(ctx, after); err != nil {
		t.Fatal(err)
	}

	keyIDs := envelopeKeyIDs(t, root)
	wantIDs := []string{provider.initial.ID, provider.initial.ID, provider.rotated.ID}
	sort.Strings(wantIDs)
	if !reflect.DeepEqual(keyIDs, wantIDs) {
		t.Fatalf("transaction key identities = %q, want %q", keyIDs, wantIDs)
	}
	restarted, err := rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []rollbackstore.Record{before, retainedBefore, after} {
		got, err := restarted.Load(ctx, record.Address, record.ArtifactDigest, record.Attempt)
		if err != nil || !bytes.Equal(got, record.Payload) {
			t.Fatalf("restart load %q = %q, %v", record.Address, got, err)
		}
	}
	if provider.historicalLoads == 0 {
		t.Fatal("pre-rotation record was not resolved through decrypt-only history")
	}
	if provider.rotateCalls != 1 {
		t.Fatalf("Rotate calls = %d, want 1", provider.rotateCalls)
	}
}

func TestStoreBlocksWhenReferencedHistoricalKeyIsUnavailable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	provider := newRotatingTestKeyProvider()
	store, err := rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	record := rollbackstore.Record{
		Address: "base/historical", ArtifactDigest: "sha256:historical", Attempt: 1,
		Payload: []byte("must not be orphaned"), Armed: true,
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RotateKey(ctx); err != nil {
		t.Fatal(err)
	}
	provider.failHistorical = errors.New("historical decrypt-only key unavailable")
	restarted, err := rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: provider})
	if restarted != nil || !errors.Is(err, rollbackstore.ErrRecoveryBlocked) || !errors.Is(err, rollbackstore.ErrKeyProtectionUnavailable) {
		t.Fatalf("restart without history = %v, %v, want blocking recovery error", restarted, err)
	}
	provider.failHistorical = nil
	restarted, err = rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Load(ctx, record.Address, record.ArtifactDigest, record.Attempt)
	if err != nil || !bytes.Equal(got, record.Payload) {
		t.Fatalf("restored historical recovery = %q, %v", got, err)
	}
}

func envelopeKeyIDs(t *testing.T, root string) []string {
	t.Helper()
	var ids []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || info.Name() != "transaction.envelope" {
			return walkErr
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var envelope struct {
			Header struct {
				Metadata struct {
					KeyID string `json:"key_id"`
				} `json:"metadata"`
			} `json:"header"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		ids = append(ids, envelope.Header.Metadata.KeyID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	return ids
}

type rotatingTestKeyProvider struct {
	initial         rollbackstore.KeyMaterial
	rotated         rollbackstore.KeyMaterial
	active          rollbackstore.KeyMaterial
	history         map[string]rollbackstore.KeyMaterial
	failHistorical  error
	historicalLoads int
	rotateCalls     int
}

func newRotatingTestKeyProvider() *rotatingTestKeyProvider {
	initial := rollbackstore.KeyMaterial{
		ID: "test-root-v1-initial", Key: bytes.Repeat([]byte{0x51}, 32), Protection: rollbackstore.ProtectionRootFile,
	}
	rotated := rollbackstore.KeyMaterial{
		ID: "test-root-v1-rotated", Key: bytes.Repeat([]byte{0x52}, 32), Protection: rollbackstore.ProtectionRootFile,
	}
	return &rotatingTestKeyProvider{
		initial: initial, rotated: rotated, active: initial,
		history: map[string]rollbackstore.KeyMaterial{initial.ID: initial},
	}
}

func (p *rotatingTestKeyProvider) LoadOrCreate(context.Context, string) (rollbackstore.KeyMaterial, error) {
	return cloneKeyMaterial(p.active), nil
}

func (p *rotatingTestKeyProvider) LoadByID(_ context.Context, _ string, id string) (rollbackstore.KeyMaterial, error) {
	p.historicalLoads++
	if p.failHistorical != nil {
		return rollbackstore.KeyMaterial{}, p.failHistorical
	}
	material, ok := p.history[id]
	if !ok {
		return rollbackstore.KeyMaterial{}, os.ErrNotExist
	}
	return cloneKeyMaterial(material), nil
}

func (p *rotatingTestKeyProvider) Rotate(context.Context, string) (rollbackstore.KeyMaterial, error) {
	p.rotateCalls++
	p.history[p.active.ID] = p.active
	p.active = p.rotated
	p.history[p.active.ID] = p.active
	return cloneKeyMaterial(p.active), nil
}

func cloneKeyMaterial(material rollbackstore.KeyMaterial) rollbackstore.KeyMaterial {
	material.Key = append([]byte(nil), material.Key...)
	return material
}
