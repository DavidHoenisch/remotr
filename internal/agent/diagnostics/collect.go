package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

const dmesgMaxLines = 5000

type Manifest = diagcatalog.BundleManifest

// Bundle is a compressed diagnostic archive and its digest.
type Bundle struct {
	Data   []byte
	SHA256 string
	Size   int64
}

// Options configures collection.
type Options struct {
	Spec         diagcatalog.Spec
	RequestID    string
	AgentVersion string
	StateDir     string
	Runner       CommandRunner
	SysReader    gosysinfo.SysReader
}

// Collect builds a tar.gz diagnostic bundle.
func Collect(ctx context.Context, opts Options) (Bundle, error) {
	if opts.Runner == nil {
		opts.Runner = DefaultRunner
	}
	if opts.SysReader == nil {
		opts.SysReader = gosysinfo.Reader{}
	}
	files := make(map[string][]byte)
	var fileNames []string

	addFile := func(name string, data []byte) error {
		name = strings.TrimPrefix(name, "/")
		if len(data) > diagcatalog.MaxCollectorBytes {
			data = append(data[:diagcatalog.MaxCollectorBytes], []byte("\n\n[truncated]\n")...)
		}
		total := int64(len(data))
		for _, existing := range files {
			total += int64(len(existing))
		}
		if total > diagcatalog.MaxBundleBytes {
			return fmt.Errorf("bundle size limit exceeded")
		}
		files[name] = data
		fileNames = append(fileNames, name)
		return nil
	}
	addSourceSummary := func(name string, data []byte, collected bool) error {
		summary, err := classifiedSourceSummary(data, collected)
		if err != nil {
			return err
		}
		return addFile(name, summary)
	}

	since := opts.Spec.Since.UTC().Format(time.RFC3339)
	until := opts.Spec.Until.UTC().Format(time.RFC3339)

	for _, collector := range opts.Spec.Collectors {
		switch collector {
		case diagcatalog.CollectorSystemInfo:
			snap := inventory.Collect(opts.SysReader)
			raw, err := inventory.MarshalJSON(snap)
			if err != nil {
				return Bundle{}, err
			}
			if err := addSourceSummary("system_info.summary.json", raw, true); err != nil {
				return Bundle{}, err
			}
		case diagcatalog.CollectorNetworkState:
			for _, item := range []struct {
				path string
				args []string
			}{
				{"network/ip-link.txt", []string{"link"}},
				{"network/ip-route.txt", []string{"route"}},
				{"network/ip-4-addr.txt", []string{"-4", "addr"}},
				{"network/ip-6-addr.txt", []string{"-6", "addr"}},
			} {
				out, runErr := opts.Runner.Run(ctx, "ip", item.args...)
				if runErr != nil {
					out = nil
				}
				if err := addSourceSummary(strings.TrimSuffix(item.path, ".txt")+".summary.json", out, runErr == nil); err != nil {
					return Bundle{}, err
				}
			}
		case diagcatalog.CollectorJournalRemotr:
			out, runErr := opts.Runner.Run(ctx, "journalctl", "-u", "remotr-agent", "--since", since, "--until", until, "-o", "short-iso", "--no-pager")
			if runErr != nil {
				out = nil
			}
			if err := addSourceSummary("journal/remotr-agent.summary.json", out, runErr == nil); err != nil {
				return Bundle{}, err
			}
		case diagcatalog.CollectorJournalKernel:
			out, runErr := opts.Runner.Run(ctx, "journalctl", "-k", "--since", since, "--until", until, "--no-pager")
			if runErr != nil {
				out = nil
			}
			if err := addSourceSummary("journal/kernel.summary.json", out, runErr == nil); err != nil {
				return Bundle{}, err
			}
		case diagcatalog.CollectorJournalAudit:
			out, runErr := opts.Runner.Run(ctx, "journalctl", "-t", "audit", "--since", since, "--until", until, "--no-pager")
			collected := runErr == nil && len(bytes.TrimSpace(out)) > 0
			if !collected {
				out = nil
			}
			if err := addSourceSummary("journal/audit.summary.json", out, collected); err != nil {
				return Bundle{}, err
			}
		case diagcatalog.CollectorDmesg:
			out, runErr := opts.Runner.Run(ctx, "dmesg", "-T")
			if runErr != nil {
				out = nil
			} else {
				out = truncateLines(out, dmesgMaxLines)
			}
			if err := addSourceSummary("kernel/dmesg.summary.json", out, runErr == nil); err != nil {
				return Bundle{}, err
			}
		case diagcatalog.CollectorRemotrAgentState:
			path := filepath.Join(opts.StateDir, "state.json")
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				raw = nil
			}
			if err := addSourceSummary("remotr/state.summary.json", raw, readErr == nil); err != nil {
				return Bundle{}, err
			}
		}
	}

	manifest := Manifest{
		RequestID:    opts.RequestID,
		AgentVersion: opts.AgentVersion,
		Collectors:   opts.Spec.Collectors,
		Since:        opts.Spec.Since,
		Until:        opts.Spec.Until,
		CollectedAt:  time.Now().UTC(),
		Files:        fileNames,
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Bundle{}, err
	}
	files["manifest.json"] = manifestRaw

	data, err := buildTarGz(files)
	if err != nil {
		return Bundle{}, err
	}
	if err := diagcatalog.ValidateBundle(data); err != nil {
		return Bundle{}, err
	}
	sum := sha256.Sum256(data)
	return Bundle{
		Data:   data,
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(data)),
	}, nil
}

func classifiedSourceSummary(data []byte, collected bool) ([]byte, error) {
	digest := sha256.Sum256(data)
	lineCount := 0
	if len(data) > 0 {
		lineCount = bytes.Count(data, []byte("\n"))
		if data[len(data)-1] != '\n' {
			lineCount++
		}
	}
	summary, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "bytes", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: intPointer(len(data))},
		{Path: "collected", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: boolPointer(collected)},
		{Path: "lines", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: intPointer(lineCount)},
		{Path: "sha256", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: hex.EncodeToString(digest[:])},
	})
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(summary, "", "  ")
}

func intPointer(value int) *int    { return &value }
func boolPointer(value bool) *bool { return &value }

func truncateLines(data []byte, max int) []byte {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) <= max {
		return data
	}
	lines = lines[:max]
	lines = append(lines, []byte("[truncated]"))
	return bytes.Join(lines, []byte("\n"))
}

func buildTarGz(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		data := files[name]
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(data)),
		}); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
		if len(data) > 0 {
			if _, err := tw.Write(data); err != nil {
				_ = tw.Close()
				_ = gz.Close()
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
