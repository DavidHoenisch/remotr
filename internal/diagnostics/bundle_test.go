package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
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

func TestValidateBundleRejectsInvalidContainerBoundaries(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if err := ValidateBundle(nil); err == nil || !strings.Contains(err.Error(), "size is invalid") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
	t.Run("over maximum", func(t *testing.T) {
		if err := ValidateBundle(make([]byte, MaxBundleBytes+1)); err == nil || !strings.Contains(err.Error(), "size is invalid") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
	t.Run("invalid gzip", func(t *testing.T) {
		if err := ValidateBundle([]byte("not-gzip")); err == nil || !strings.Contains(err.Error(), "diagnostic bundle gzip") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
	t.Run("invalid tar", func(t *testing.T) {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if _, err := writer.Write([]byte("not-a-tar")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBundle(buffer.Bytes()); err == nil || !strings.Contains(err.Error(), "diagnostic bundle tar") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
	t.Run("too many entries", func(t *testing.T) {
		entries := make([]testBundleEntry, maxBundleEntries)
		for i := range entries {
			entries[i] = testBundleEntry{name: "entry-" + strings.Repeat("x", i+1)}
		}
		if err := ValidateBundle(testBundleEntriesArchive(t, entries)); err == nil || !strings.Contains(err.Error(), "too many entries") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
	t.Run("oversized entry", func(t *testing.T) {
		archive := testBundleEntriesArchive(t, []testBundleEntry{{name: "manifest.json", content: make([]byte, MaxCollectorBytes+1)}})
		if err := ValidateBundle(archive); err == nil || !strings.Contains(err.Error(), "entry exceeds its limit") {
			t.Fatalf("ValidateBundle() error = %v", err)
		}
	})
}

func TestValidateBundleRejectsUnsafeAndDuplicateTarEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []testBundleEntry
		want    string
	}{
		{name: "absolute", entries: []testBundleEntry{{name: "/manifest.json"}}, want: "entry is unsafe"},
		{name: "traversal", entries: []testBundleEntry{{name: "../manifest.json"}}, want: "entry is unsafe"},
		{name: "directory", entries: []testBundleEntry{{name: "manifest.json", typeflag: tar.TypeDir}}, want: "entry is unsafe"},
		{name: "duplicate", entries: []testBundleEntry{{name: "manifest.json"}, {name: "manifest.json"}}, want: "entry is duplicated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateBundle(testBundleEntriesArchive(t, tt.entries)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateBundle() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateBundleRejectsInvalidManifestContract(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BundleManifest)
	}{
		{name: "missing request", mutate: func(manifest *BundleManifest) { manifest.RequestID = "" }},
		{name: "long request", mutate: func(manifest *BundleManifest) { manifest.RequestID = strings.Repeat("x", 129) }},
		{name: "unsafe request", mutate: func(manifest *BundleManifest) { manifest.RequestID = "request/1" }},
		{name: "unsafe agent version", mutate: func(manifest *BundleManifest) { manifest.AgentVersion = "version/1" }},
		{name: "missing since", mutate: func(manifest *BundleManifest) { manifest.Since = time.Time{} }},
		{name: "missing until", mutate: func(manifest *BundleManifest) { manifest.Until = time.Time{} }},
		{name: "missing collected time", mutate: func(manifest *BundleManifest) { manifest.CollectedAt = time.Time{} }},
		{name: "unknown collector", mutate: func(manifest *BundleManifest) { manifest.Collectors = []string{"unknown"} }},
		{name: "duplicate collector", mutate: func(manifest *BundleManifest) {
			manifest.Collectors = []string{CollectorJournalRemotr, CollectorJournalRemotr}
		}},
		{name: "unexpected file list", mutate: func(manifest *BundleManifest) { manifest.Files = []string{"raw.log"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := validTestBundleFiles(t)
			mutateTestManifest(t, files, tt.mutate)
			if err := ValidateBundle(testBundleArchive(t, files)); err == nil {
				t.Fatal("ValidateBundle() error = nil")
			}
		})
	}

	t.Run("unknown manifest field", func(t *testing.T) {
		files := validTestBundleFiles(t)
		var manifest map[string]any
		if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		manifest["raw"] = "secret-canary"
		var err error
		files["manifest.json"], err = json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateBundle(testBundleArchive(t, files)); err == nil {
			t.Fatal("ValidateBundle() accepted unknown manifest field")
		}
	})

	t.Run("malformed manifest", func(t *testing.T) {
		files := validTestBundleFiles(t)
		files["manifest.json"] = []byte(`{"requestId":`)
		if err := ValidateBundle(testBundleArchive(t, files)); err == nil {
			t.Fatal("ValidateBundle() accepted malformed manifest")
		}
	})
}

func TestValidateBundleRejectsInvalidClassifiedSourceSummaries(t *testing.T) {
	present := true
	count := 1
	digest := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		fields []executor.SafeField
		raw    []byte
	}{
		{name: "malformed JSON", raw: []byte(`{"fields":`)},
		{name: "too few fields", fields: []executor.SafeField{
			{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count},
			{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &present},
			{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: digest},
		}},
		{name: "duplicate field", fields: []executor.SafeField{
			{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count},
			{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count},
			{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &present},
			{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: digest},
		}},
		{name: "bytes wrong projection", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &present})},
		{name: "collected wrong projection", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count})},
		{name: "fingerprint wrong sensitivity", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "sha256", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: digest})},
		{name: "fingerprint wrong projection", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: digest})},
		{name: "fingerprint invalid hex", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: strings.Repeat("z", 64)})},
		{name: "fingerprint wrong length", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: "aa"})},
		{name: "fingerprint overlong", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: strings.Repeat("aa", 33)})},
		{name: "unknown field", fields: testSourceSummaryFields(&count, &present, digest, executor.SafeField{Path: "unknown", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: "safe"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := validTestBundleFiles(t)
			content := tt.raw
			if content == nil {
				summary, err := executor.NewSafeSummary(tt.fields)
				if err != nil {
					t.Fatal(err)
				}
				content, err = json.Marshal(summary)
				if err != nil {
					t.Fatal(err)
				}
			}
			files["journal/remotr-agent.summary.json"] = content
			if err := ValidateBundle(testBundleArchive(t, files)); err == nil {
				t.Fatal("ValidateBundle() accepted invalid classified summary")
			}
		})
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

func mutateTestManifest(t *testing.T, files map[string][]byte, mutate func(*BundleManifest)) {
	t.Helper()
	var manifest BundleManifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	var err error
	files["manifest.json"], err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
}

func testSourceSummaryFields(count *int, present *bool, digest string, replacement executor.SafeField) []executor.SafeField {
	fields := []executor.SafeField{
		{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: count},
		{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: present},
		{Path: "lines", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: count},
		{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: digest},
	}
	for i := range fields {
		if fields[i].Path == replacement.Path || replacement.Path == "unknown" && fields[i].Path == "lines" {
			fields[i] = replacement
			break
		}
	}
	return fields
}

type testBundleEntry struct {
	name     string
	typeflag byte
	content  []byte
}

func testBundleEntriesArchive(t *testing.T, entries []testBundleEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: 0o600, Size: int64(len(entry.content)), Typeflag: typeflag}
		if typeflag == tar.TypeDir {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := tarWriter.Write(entry.content); err != nil {
				t.Fatal(err)
			}
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
