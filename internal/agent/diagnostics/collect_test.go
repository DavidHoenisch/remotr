package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

type stubRunner struct {
	outputs map[string][]byte
}

func TestCollectProjectsSecretBearingSourcesIntoClassifiedMetadata(t *testing.T) {
	const canary = "diagnostic-source-secret-canary"
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"token":"`+canary+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	bundle, err := Collect(t.Context(), Options{
		Spec: diagcatalog.Spec{
			Collectors: []string{diagcatalog.CollectorJournalRemotr, diagcatalog.CollectorRemotrAgentState},
			Since:      now.Add(-time.Hour), Until: now,
		},
		RequestID: "request", StateDir: stateDir,
		Runner: stubRunner{outputs: map[string][]byte{"journalctl -u": []byte("message " + canary + "\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := readDiagnosticArchive(t, bundle.Data)
	for name, content := range files {
		if strings.Contains(string(content), canary) {
			t.Fatalf("diagnostic file %q leaked canary: %s", name, content)
		}
	}
	for _, name := range []string{"journal/remotr-agent.summary.json", "remotr/state.summary.json"} {
		content, ok := files[name]
		if !ok {
			t.Fatalf("classified diagnostic file %q missing: %v", name, files)
		}
		var summary executor.SafeSummary
		if err := json.Unmarshal(content, &summary); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if err := summary.Validate(); err != nil || !strings.Contains(summary.String(), "sha256=") || !strings.Contains(summary.String(), "bytes=") {
			t.Fatalf("classified summary %q = %+v, err=%v", name, summary, err)
		}
	}
}

func readDiagnosticArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := make(map[string][]byte)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = content
	}
}

func (s stubRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}
	if out, ok := s.outputs[key]; ok {
		return out, nil
	}
	return []byte("ok\n"), nil
}

func TestCollect_buildsBundleWithManifest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	bundle, err := Collect(context.Background(), Options{
		Spec: diagcatalog.Spec{
			Collectors: []string{diagcatalog.CollectorNetworkState},
			Since:      now.Add(-time.Hour),
			Until:      now,
		},
		RequestID: "req-1",
		Runner: stubRunner{outputs: map[string][]byte{
			"ip link": []byte("1: lo\n"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Size == 0 || bundle.SHA256 == "" {
		t.Fatalf("bundle = %+v", bundle)
	}

	gz, err := gzip.NewReader(bytes.NewReader(bundle.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		if _, err := tr.Next(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("invalid tar archive: %v", err)
		}
	}
}
