package rollbackstore

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Task 1.3 migration baseline at the system-safety recovery seam. The golden
// values were produced independently from rollbackstore.Store and freeze the
// split payload/metadata format that predates transaction envelopes.
func TestLegacySplitRecordCompatibilityFixture(t *testing.T) {
	fixture := readLegacySplitRecordFixture(t)
	if fixture.Format != "rollbackstore-split-record/v0" || fixture.ValidDisposition != "migrate" || fixture.TamperedDisposition != "block_affected_resource" {
		t.Fatalf("fixture compatibility contract = %+v", fixture)
	}

	t.Run("valid record remains recoverable", func(t *testing.T) {
		root, expected := installLegacySplitRecordFixture(t, fixture)
		store, err := New(Options{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(context.Background(), fixture.Address, fixture.ArtifactDigest, fixture.Attempt)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(expected) {
			t.Fatalf("legacy payload = %q, want %q", got, expected)
		}
		var envelopes int
		if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			switch info.Name() {
			case "transaction.envelope":
				envelopes++
			case "metadata.json", "payload.bin":
				t.Fatalf("legacy split file survived migration: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if envelopes != 1 {
			t.Fatalf("migrated envelope count = %d, want 1", envelopes)
		}
	})

	t.Run("tampered record blocks migration", func(t *testing.T) {
		root, expected := installLegacySplitRecordFixture(t, fixture)
		path := legacyPayloadPath(root, fixture)
		ciphertext, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ciphertext[0] ^= 0xff
		if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
			t.Fatal(err)
		}
		store, err := New(Options{Root: root})
		if store != nil || !errors.Is(err, ErrRecoveryBlocked) {
			t.Fatalf("tampered legacy startup returned store=%v, error=%v", store, err)
		}
		if errors.Is(err, os.ErrNotExist) || bytes.Contains([]byte(err.Error()), expected) {
			t.Fatalf("tampered legacy record returned unsafe or misleading error %q", err)
		}
	})
}

func TestVersionOneTransactionEnvelopeRemainsRecoverable(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	store, err := New(Options{Root: root, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("version-one-envelope-payload")
	digest := sha256.Sum256(payload)
	block, err := aes.NewCipher(store.key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	meta := metadata{
		Version: RecordVersion, State: LifecycleArmed,
		Address: "base/legacy-envelope", ArtifactDigest: "sha256:legacy-envelope", Attempt: 1,
		CreatedAt: now, Armed: true,
		Sensitive: true, ExpiresAt: now.Add(time.Hour), KeyID: store.keyID,
		Nonce: bytes.Repeat([]byte{0x7a}, gcm.NonceSize()), Checksum: hex.EncodeToString(digest[:]), PayloadPresent: true,
	}
	header := envelopeHeader{Version: legacyEnvelopeVersion, Metadata: meta}
	aad, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(transactionEnvelope{Header: header, Ciphertext: gcm.Seal(nil, meta.Nonce, payload, aad)})
	if err != nil {
		t.Fatal(err)
	}
	key := recordKey{Address: meta.Address, ArtifactDigest: meta.ArtifactDigest, Attempt: meta.Attempt}
	if err := store.writeEnvelopeAtomic(store.recordDir(key.Address, key.ArtifactDigest, key.Attempt), raw); err != nil {
		t.Fatal(err)
	}

	restarted, err := New(Options{Root: root, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.Load(context.Background(), key.Address, key.ArtifactDigest, key.Attempt)
	if err != nil || !bytes.Equal(recovered, payload) {
		t.Fatalf("version-one envelope recovery = %q, %v", recovered, err)
	}
	if err := restarted.markRolledBack(key); err != nil {
		t.Fatal(err)
	}
	upgraded, err := os.ReadFile(restarted.envelopePath(key))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(upgraded, []byte(`"checksum"`)) || bytes.Contains(upgraded, []byte(hex.EncodeToString(digest[:]))) {
		t.Fatalf("rewritten envelope retained legacy payload fingerprint: %s", upgraded)
	}
}

type legacySplitRecordFixture struct {
	Format                string          `json:"format"`
	KeyHex                string          `json:"key_hex"`
	Address               string          `json:"address"`
	ArtifactDigest        string          `json:"artifact_digest"`
	Attempt               int             `json:"attempt"`
	Metadata              json.RawMessage `json:"metadata"`
	CiphertextBase64      string          `json:"ciphertext_base64"`
	ExpectedPayloadBase64 string          `json:"expected_payload_base64"`
	ValidDisposition      string          `json:"expected_valid_disposition"`
	TamperedDisposition   string          `json:"expected_tampered_disposition"`
}

func readLegacySplitRecordFixture(t *testing.T) legacySplitRecordFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy-split-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture legacySplitRecordFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func installLegacySplitRecordFixture(t *testing.T, fixture legacySplitRecordFixture) (string, []byte) {
	t.Helper()
	key, err := hex.DecodeString(fixture.KeyHex)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(fixture.CiphertextBase64)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := base64.StdEncoding.DecodeString(fixture.ExpectedPayloadBase64)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rollback.key"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(legacyPayloadPath(root, fixture))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), fixture.Metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload.bin"), ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, expected
}

func legacyPayloadPath(root string, fixture legacySplitRecordFixture) string {
	address := sha256.Sum256([]byte(fixture.Address))
	digest := sha256.Sum256([]byte(fixture.ArtifactDigest))
	return filepath.Join(root, "records", hex.EncodeToString(address[:]), hex.EncodeToString(digest[:]), strconv.Itoa(fixture.Attempt), "payload.bin")
}
