package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

const maxBundleEntries = 32

// BundleManifest describes the classified metadata-only diagnostic artifact.
type BundleManifest struct {
	RequestID    string    `json:"requestId"`
	AgentVersion string    `json:"agentVersion,omitempty"`
	Collectors   []string  `json:"collectors"`
	Since        time.Time `json:"since"`
	Until        time.Time `json:"until"`
	CollectedAt  time.Time `json:"collectedAt"`
	Files        []string  `json:"files"`
}

// ValidateBundle rejects any diagnostic archive that contains raw collector
// bytes or a field outside the closed classified-summary format.
func ValidateBundle(data []byte) error {
	if len(data) == 0 || len(data) > MaxBundleBytes {
		return errors.New("diagnostic bundle size is invalid")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("diagnostic bundle gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := make(map[string][]byte)
	for entries := 0; ; entries++ {
		if entries >= maxBundleEntries {
			return errors.New("diagnostic bundle has too many entries")
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("diagnostic bundle tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg || header.Name == "" || strings.HasPrefix(header.Name, "/") || strings.Contains(header.Name, "..") {
			return errors.New("diagnostic bundle entry is unsafe")
		}
		if _, duplicate := files[header.Name]; duplicate {
			return errors.New("diagnostic bundle entry is duplicated")
		}
		content, err := io.ReadAll(io.LimitReader(tr, MaxCollectorBytes+1))
		if err != nil || len(content) > MaxCollectorBytes {
			return errors.New("diagnostic bundle entry exceeds its limit")
		}
		files[header.Name] = content
	}
	manifestRaw, ok := files["manifest.json"]
	if !ok {
		return errors.New("diagnostic bundle manifest is missing")
	}
	delete(files, "manifest.json")
	var manifest BundleManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("diagnostic bundle manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("diagnostic bundle manifest has trailing data")
	}
	if !safeBundleIdentifier(manifest.RequestID) || (manifest.AgentVersion != "" && !safeBundleIdentifier(manifest.AgentVersion)) || manifest.Since.IsZero() || manifest.Until.IsZero() || manifest.CollectedAt.IsZero() {
		return errors.New("diagnostic bundle manifest identity is invalid")
	}
	collectors, err := NormalizeCollectors(manifest.Collectors)
	if err != nil || !slices.Equal(collectors, manifest.Collectors) {
		return errors.New("diagnostic bundle collectors are invalid")
	}
	expected := expectedSummaryFiles(collectors)
	listed := append([]string(nil), manifest.Files...)
	slices.Sort(expected)
	slices.Sort(listed)
	if !slices.Equal(expected, listed) || len(files) != len(expected) {
		return errors.New("diagnostic bundle files do not match its collectors")
	}
	for _, name := range expected {
		content, ok := files[name]
		if !ok {
			return fmt.Errorf("diagnostic bundle summary %q is missing", name)
		}
		if err := validateSourceSummary(content); err != nil {
			return fmt.Errorf("diagnostic bundle summary %q: %w", name, err)
		}
	}
	return nil
}

func safeBundleIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._:-+", char) {
			continue
		}
		return false
	}
	return true
}

func expectedSummaryFiles(collectors []string) []string {
	var files []string
	for _, collector := range collectors {
		switch collector {
		case CollectorSystemInfo:
			files = append(files, "system_info.summary.json")
		case CollectorNetworkState:
			files = append(files, "network/ip-link.summary.json", "network/ip-route.summary.json", "network/ip-4-addr.summary.json", "network/ip-6-addr.summary.json")
		case CollectorJournalRemotr:
			files = append(files, "journal/remotr-agent.summary.json")
		case CollectorJournalKernel:
			files = append(files, "journal/kernel.summary.json")
		case CollectorJournalAudit:
			files = append(files, "journal/audit.summary.json")
		case CollectorDmesg:
			files = append(files, "kernel/dmesg.summary.json")
		case CollectorRemotrAgentState:
			files = append(files, "remotr/state.summary.json")
		}
	}
	return files
}

func validateSourceSummary(content []byte) error {
	var summary executor.SafeSummary
	if err := json.Unmarshal(content, &summary); err != nil {
		return err
	}
	if err := summary.Validate(); err != nil || len(summary.Fields) != 4 {
		return errors.New("classified source summary has an invalid shape")
	}
	seen := make(map[string]bool, 4)
	for _, field := range summary.Fields {
		if seen[field.Path] {
			return errors.New("classified source summary has a duplicate field")
		}
		seen[field.Path] = true
		switch field.Path {
		case "bytes", "lines":
			if field.Sensitivity != executor.SafeSensitiveMetadata || field.Projection != executor.SafeCount || field.Count == nil || *field.Count < 0 {
				return errors.New("classified source count is invalid")
			}
		case "collected":
			if field.Sensitivity != executor.SafeSensitiveMetadata || field.Projection != executor.SafePresence || field.Present == nil {
				return errors.New("classified source presence is invalid")
			}
		case "sha256":
			decoded, err := hex.DecodeString(field.Text)
			if field.Sensitivity != executor.SafeSensitiveMetadata || field.Projection != executor.SafeFingerprint || err != nil || len(decoded) != 32 {
				return errors.New("classified source fingerprint is invalid")
			}
		default:
			return errors.New("classified source field is not allowed")
		}
	}
	return nil
}
