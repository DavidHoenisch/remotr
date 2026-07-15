package rollbackstore_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

var benchmarkRollbackPayload []byte

func BenchmarkRollbackStore(b *testing.B) {
	ctx := context.Background()
	for _, size := range []int{256, 4 << 10, 64 << 10} {
		payload := bytes.Repeat([]byte{0x5a}, size)
		store := newBenchmarkStore(b, b.TempDir(), func() time.Time { return time.Unix(0, 0).UTC() })
		record := rollbackstore.Record{Address: "benchmark/resource", ArtifactDigest: "sha256:benchmark", Attempt: 1, Payload: payload, Armed: true}
		if err := store.Save(ctx, record); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("reserve-encrypt-save/bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.Save(ctx, record); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("load-decrypt/bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				benchmarkRollbackPayload, err = store.Load(ctx, record.Address, record.ArtifactDigest, record.Attempt)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	for _, staleCount := range []int{10, 100} {
		b.Run(fmt.Sprintf("prune-cleanup/stale=%d", staleCount), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				now := time.Unix(0, 0).UTC()
				store := newBenchmarkStore(b, b.TempDir(), func() time.Time { return now })
				for attempt := 1; attempt <= staleCount; attempt++ {
					record := rollbackstore.Record{Address: "benchmark/stale", ArtifactDigest: "sha256:stale", Attempt: attempt, Payload: []byte("rollback"), Armed: false}
					if err := store.Save(ctx, record); err != nil {
						b.Fatal(err)
					}
				}
				now = now.Add(2 * time.Hour)
				trigger := rollbackstore.Record{Address: "benchmark/current", ArtifactDigest: "sha256:current", Attempt: 1, Payload: []byte("current"), Armed: true}
				b.StartTimer()
				if err := store.Save(ctx, trigger); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newBenchmarkStore(b testing.TB, root string, now func() time.Time) *rollbackstore.Store {
	b.Helper()
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: root, MaxBytes: 128 << 20, MaxAttempts: 1000, MaxAge: time.Hour, Now: now,
		KeyProvider: benchmarkRollbackKeyProvider{},
	})
	if err != nil {
		b.Fatal(err)
	}
	return store
}

type benchmarkRollbackKeyProvider struct{}

func (benchmarkRollbackKeyProvider) LoadOrCreate(context.Context, string) ([]byte, error) {
	return bytes.Repeat([]byte{0x24}, 32), nil
}
