package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"
	"time"

	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
)

type stubRunner struct {
	outputs map[string][]byte
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
