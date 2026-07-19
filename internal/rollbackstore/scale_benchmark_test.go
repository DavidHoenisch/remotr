package rollbackstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkTransactionResourceCount int

// BenchmarkTransactionReservationRecovery measures the public capacity claim
// and restart-recovery paths over the shared representative resource counts.
// Recovery fixture construction and restart scanning stay outside the timed
// region; the timed operation decrypts, calls the recovery boundary, and
// durably transitions every armed transaction.
func BenchmarkTransactionReservationRecovery(b *testing.B) {
	ctx := context.Background()
	payload := bytes.Repeat([]byte{0x5a}, 256)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	for _, size := range benchmarkfixture.Sizes() {
		resourceCount := int(size)
		requests := make([]ReservationRequest, resourceCount)
		for index := range requests {
			requests[index] = ReservationRequest{
				Address:        fmt.Sprintf("benchmark/resource-%04d", index),
				ArtifactDigest: "sha256:benchmark", Attempt: 1,
				PayloadBytes: int64(len(payload)),
			}
		}

		b.Run("reserve/resources="+size.String(), func(b *testing.B) {
			store := newScaleBenchmarkStore(b, b.TempDir(), now, resourceCount)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				reservations := make([]*Reservation, 0, resourceCount)
				for _, request := range requests {
					reservation, err := store.Reserve(ctx, request)
					if err != nil {
						b.Fatal(err)
					}
					reservations = append(reservations, reservation)
				}
				for _, reservation := range reservations {
					reservation.Release()
				}
				benchmarkTransactionResourceCount = len(reservations)
			}
		})

		b.Run("recover/resources="+size.String(), func(b *testing.B) {
			root := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				iterationRoot := filepath.Join(root, fmt.Sprintf("iteration-%d", iteration))
				options := scaleBenchmarkOptions(iterationRoot, now, resourceCount)
				seed, err := New(options)
				if err != nil {
					b.Fatal(err)
				}
				for index := 0; index < resourceCount; index++ {
					if err := seedScaleBenchmarkRecord(seed, now, index, payload); err != nil {
						b.Fatal(err)
					}
				}
				restarted, err := New(options)
				if err != nil {
					b.Fatal(err)
				}
				recovered := 0
				b.StartTimer()
				err = restarted.RecoverArmed(ctx, func(context.Context, Recovery) error {
					recovered++
					return nil
				})
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				if recovered != resourceCount {
					b.Fatalf("recovered %d resources, want %d", recovered, resourceCount)
				}
				benchmarkTransactionResourceCount = recovered
			}
		})
	}
}

func newScaleBenchmarkStore(b testing.TB, root string, now time.Time, resourceCount int) *Store {
	b.Helper()
	store, err := New(scaleBenchmarkOptions(root, now, resourceCount))
	if err != nil {
		b.Fatal(err)
	}
	return store
}

func scaleBenchmarkOptions(root string, now time.Time, resourceCount int) Options {
	return Options{
		Root: root, MaxBytes: 1 << 30, MaxAttempts: resourceCount + 1,
		MaxAge: 30 * 24 * time.Hour, FilesystemAllowance: 1,
		Now: func() time.Time { return now }, KeyProvider: scaleBenchmarkKeyProvider{},
		AvailableBytes: func(string) (int64, error) { return 1 << 30, nil },
	}
}

func seedScaleBenchmarkRecord(store *Store, now time.Time, index int, payload []byte) error {
	address := fmt.Sprintf("benchmark/resource-%04d", index)
	meta := metadata{
		Version: RecordVersion, State: LifecycleArmed,
		Address: address, ArtifactDigest: "sha256:benchmark", Attempt: 1,
		CreatedAt: now, Armed: true,
	}
	encoded, err := store.sealEnvelope(meta, payload, true)
	if err != nil {
		return err
	}
	dir := store.recordDir(address, meta.ArtifactDigest, meta.Attempt)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, envelopeFilename), encoded, 0o600)
}

type scaleBenchmarkKeyProvider struct{}

func (scaleBenchmarkKeyProvider) LoadOrCreate(context.Context, string) (KeyMaterial, error) {
	return KeyMaterial{
		ID: "scale-benchmark-v1", Key: bytes.Repeat([]byte{0x42}, 32),
		Protection: ProtectionRootFile,
	}, nil
}
