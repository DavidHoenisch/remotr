package rollbackstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
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
