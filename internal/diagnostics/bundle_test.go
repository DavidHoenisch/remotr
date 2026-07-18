package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

func TestValidateBundleRejectsTrailingManifestData(t *testing.T) {
	files := validTestBundleFiles(t)
	files["manifest.json"] = append(files["manifest.json"], []byte(`{"unexpected":"document"}`)...)

	if err := ValidateBundle(testBundleArchive(t, files)); err == nil {
		t.Fatal("ValidateBundle() error = nil, want trailing manifest data rejected")
	}
}

func TestValidateBundleAcceptsClassifiedMetadataOnlyArchive(t *testing.T) {
	if err := ValidateBundle(testBundleArchive(t, validTestBundleFiles(t))); err != nil {
		t.Fatalf("ValidateBundle() error = %v", err)
	}
}

func TestValidateBundleRejectsRawCollectorEntry(t *testing.T) {
	const canary = "diagnostic-raw-entry-secret-canary"
	files := validTestBundleFiles(t)
	files["journal/remotr-agent.log"] = []byte(canary)

	err := ValidateBundle(testBundleArchive(t, files))
	if err == nil {
		t.Fatal("ValidateBundle() error = nil, want raw collector entry rejected")
	}
	if bytes.Contains([]byte(err.Error()), []byte(canary)) {
		t.Fatalf("ValidateBundle() error leaked raw collector bytes: %v", err)
	}
}

func validTestBundleFiles(t *testing.T) map[string][]byte {
	t.Helper()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	manifest, err := json.Marshal(BundleManifest{
		RequestID:   "request-1",
		Collectors:  []string{CollectorJournalRemotr},
		Since:       now.Add(-time.Hour),
		Until:       now,
		CollectedAt: now,
		Files:       []string{"journal/remotr-agent.summary.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("source bytes"))
	byteCount := 12
	lineCount := 1
	collected := true
	summary, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &byteCount},
		{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &collected},
		{Path: "lines", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &lineCount},
		{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: hex.EncodeToString(digest[:])},
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{
		"manifest.json":                     manifest,
		"journal/remotr-agent.summary.json": summaryJSON,
	}
}

func testBundleArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
