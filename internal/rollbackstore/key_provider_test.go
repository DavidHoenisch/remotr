package rollbackstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-070: an endpoint without a supported TPM uses an explicitly reduced
// root-file protection class and reports only safe key identity metadata.
func TestStoreSelectsExplicitRootFallbackAndReportsReducedProtection(t *testing.T) {
	root := t.TempDir()
	key := []byte(testsupport.SecretCanary("root-fallback-key-material"))[:32]
	rootProvider := &recordingKeyProvider{material: rollbackstore.KeyMaterial{
		ID: "root-v1-safe-identity", Key: key, Protection: rollbackstore.ProtectionRootFile,
	}}
	tpmProvider := &recordingKeyProvider{err: errors.New("TPM provider must not be called")}
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: root,
		KeyProvider: &rollbackstore.CapabilityKeyProvider{
			Capability: fixedTPMCapability{supported: false},
			TPM:        tpmProvider, Root: rootProvider,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := store.Protection()
	if report.Class != rollbackstore.ProtectionRootFile || !report.ReducedProtection {
		t.Fatalf("protection report = %+v, want explicit reduced root-file class", report)
	}
	if report.KeyID != "root-v1-safe-identity" || report.Limitation != rollbackstore.RootCompromiseLimitation {
		t.Fatalf("protection report = %+v, want safe identity and root-compromise limitation", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, key) || bytes.Contains(encoded, []byte("remotr-test-secret-")) {
		t.Fatalf("protection report exposed key material: %s", encoded)
	}
	if rootProvider.calls != 1 || tpmProvider.calls != 0 {
		t.Fatalf("provider calls root=%d TPM=%d, want root=1 TPM=0", rootProvider.calls, tpmProvider.calls)
	}

	// Persisted selection is policy state. Newly appearing TPM capability must
	// not silently rotate or orphan records; explicit rotation owns that change.
	restartedRoot := &recordingKeyProvider{material: rootProvider.material}
	restartedTPM := &recordingKeyProvider{err: errors.New("unexpected silent TPM upgrade")}
	restarted, err := rollbackstore.New(rollbackstore.Options{
		Root: root,
		KeyProvider: &rollbackstore.CapabilityKeyProvider{
			Capability: fixedTPMCapability{supported: true},
			TPM:        restartedTPM, Root: restartedRoot,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Protection().Class != rollbackstore.ProtectionRootFile || restartedRoot.calls != 1 || restartedTPM.calls != 0 {
		t.Fatalf("persisted root selection was not honored: report=%+v root=%d TPM=%d",
			restarted.Protection(), restartedRoot.calls, restartedTPM.calls)
	}
}

// OS-AEC-082: after TPM selection, a provider failure is blocking and cannot
// be converted into a root-file transaction by a later capability downgrade.
func TestSelectedTPMFailureBlocksWithoutSilentRootDowngrade(t *testing.T) {
	root := t.TempDir()
	tpmFailure := errors.New("selected TPM cannot seal key")
	tpm := &recordingKeyProvider{err: tpmFailure}
	fallback := &recordingKeyProvider{material: rollbackstore.KeyMaterial{
		ID: "forbidden-root", Key: bytes.Repeat([]byte{0x11}, 32), Protection: rollbackstore.ProtectionRootFile,
	}}
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: root,
		KeyProvider: &rollbackstore.CapabilityKeyProvider{
			Capability: fixedTPMCapability{supported: true}, TPM: tpm, Root: fallback,
		},
	})
	if store != nil || !errors.Is(err, rollbackstore.ErrKeyProtectionUnavailable) || !errors.Is(err, tpmFailure) {
		t.Fatalf("selected TPM startup = %v, %v, want blocking provider failure", store, err)
	}
	if tpm.calls != 1 || fallback.calls != 0 {
		t.Fatalf("provider calls TPM=%d root=%d, want TPM=1 root=0", tpm.calls, fallback.calls)
	}

	// Even if the next probe says unsupported, the durable TPM selection wins.
	restartedTPM := &recordingKeyProvider{err: tpmFailure}
	restartedRoot := &recordingKeyProvider{material: fallback.material}
	store, err = rollbackstore.New(rollbackstore.Options{
		Root: root,
		KeyProvider: &rollbackstore.CapabilityKeyProvider{
			Capability: fixedTPMCapability{supported: false}, TPM: restartedTPM, Root: restartedRoot,
		},
	})
	if store != nil || !errors.Is(err, rollbackstore.ErrKeyProtectionUnavailable) {
		t.Fatalf("persisted TPM startup = %v, %v, want blocking provider failure", store, err)
	}
	if restartedTPM.calls != 1 || restartedRoot.calls != 0 {
		t.Fatalf("restart provider calls TPM=%d root=%d, want TPM=1 root=0", restartedTPM.calls, restartedRoot.calls)
	}
}

func TestCapabilitySelectionPreservesPreexistingProtectionState(t *testing.T) {
	t.Run("legacy root key wins over newly detected TPM", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "rollback.key"), bytes.Repeat([]byte{0x42}, 32), 0o600); err != nil {
			t.Fatal(err)
		}
		rootProvider := &recordingKeyProvider{material: rollbackstore.KeyMaterial{
			ID: "existing-root-v1", Key: bytes.Repeat([]byte{0x42}, 32), Protection: rollbackstore.ProtectionRootFile,
		}}
		tpmProvider := &recordingKeyProvider{material: rollbackstore.KeyMaterial{
			ID: "new-tpm-v1", Key: bytes.Repeat([]byte{0x43}, 32), Protection: rollbackstore.ProtectionTPMSealed,
		}}
		store, err := rollbackstore.New(rollbackstore.Options{
			Root: root,
			KeyProvider: &rollbackstore.CapabilityKeyProvider{
				Capability: fixedTPMCapability{supported: true}, TPM: tpmProvider, Root: rootProvider,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if store.Protection().Class != rollbackstore.ProtectionRootFile || rootProvider.calls != 1 || tpmProvider.calls != 0 {
			t.Fatalf("preexisting selection = %+v root=%d TPM=%d, want root=1 TPM=0",
				store.Protection(), rootProvider.calls, tpmProvider.calls)
		}
	})

	t.Run("ambiguous provider state blocks startup", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"rollback.key", "rollback.tpm-key"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("preexisting"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		provider := &recordingKeyProvider{err: errors.New("ambiguous state must block before provider call")}
		store, err := rollbackstore.New(rollbackstore.Options{
			Root: root,
			KeyProvider: &rollbackstore.CapabilityKeyProvider{
				Capability: fixedTPMCapability{supported: true}, TPM: provider, Root: provider,
			},
		})
		if store != nil || !errors.Is(err, rollbackstore.ErrKeyProtectionUnavailable) || provider.calls != 0 {
			t.Fatalf("ambiguous startup = %v, %v, calls=%d", store, err, provider.calls)
		}
	})
}

func TestRootKeyProviderPersistsVersionedRootOnlyMaterial(t *testing.T) {
	ctx := context.Background()
	t.Run("new key", func(t *testing.T) {
		root := t.TempDir()
		first, err := (rollbackstore.RootKeyProvider{}).LoadOrCreate(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := (rollbackstore.RootKeyProvider{}).LoadOrCreate(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if first.ID == "" || first.ID != second.ID || !bytes.Equal(first.Key, second.Key) {
			t.Fatalf("versioned root key did not reload: first=%q second=%q", first.ID, second.ID)
		}
		if first.Protection != rollbackstore.ProtectionRootFile || len(first.Key) != 32 {
			t.Fatalf("root key material = id %q class %q bytes %d", first.ID, first.Protection, len(first.Key))
		}
		path := filepath.Join(root, "rollback.key")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Version != 1 {
			t.Fatalf("root key file is not versioned JSON: %q, %v", raw, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("root key mode = %o, want 600", info.Mode().Perm())
		}
	})

	t.Run("legacy raw key migrates without changing material", func(t *testing.T) {
		root := t.TempDir()
		legacy := bytes.Repeat([]byte{0x5a}, 32)
		path := filepath.Join(root, "rollback.key")
		if err := os.WriteFile(path, legacy, 0o600); err != nil {
			t.Fatal(err)
		}
		material, err := (rollbackstore.RootKeyProvider{}).LoadOrCreate(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(material.Key, legacy) || material.ID == "" {
			t.Fatalf("legacy migration changed material or omitted identity: id=%q key=%x", material.ID, material.Key)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(raw) {
			t.Fatalf("legacy root key was not migrated to a versioned envelope: %q", raw)
		}
	})
}

type fixedTPMCapability struct {
	supported bool
	err       error
}

func (c fixedTPMCapability) Supported(context.Context) (bool, error) {
	return c.supported, c.err
}

type recordingKeyProvider struct {
	material rollbackstore.KeyMaterial
	err      error
	calls    int
}

func (p *recordingKeyProvider) LoadOrCreate(context.Context, string) (rollbackstore.KeyMaterial, error) {
	p.calls++
	return p.material, p.err
}
