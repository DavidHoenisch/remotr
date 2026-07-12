package configrepo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkValidationErr error
var benchmarkArtifactYAML []byte

func BenchmarkValidateState(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		raw := benchmarkfixture.Artifact(size)
		state, err := models.ParseState(bytes.NewReader(raw))
		if err != nil {
			b.Fatalf("ParseState() error = %v", err)
		}
		if err := ValidateState(state, "benchmark.yaml"); err != nil {
			b.Fatalf("ValidateState() setup error = %v", err)
		}

		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkValidationErr = ValidateState(state, "benchmark.yaml")
				if benchmarkValidationErr != nil {
					b.Fatal(benchmarkValidationErr)
				}
			}
		})
	}
}

func BenchmarkResolveArtifact(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		raw := benchmarkfixture.Artifact(size)
		root := b.TempDir()
		writeBenchmarkArtifact(b, filepath.Join(root, "fleets", "benchmark", "desired.yaml"), raw)
		writeBenchmarkArtifact(b, filepath.Join(root, "endpoints", "benchmark-endpoint", "desired.yaml"), raw)

		for _, target := range []struct {
			name       string
			endpointID string
		}{
			{"endpoint-override", "benchmark-endpoint"},
			{"fleet-fallback", "benchmark-member"},
		} {
			b.Run(target.name+"/resources="+size.String(), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(raw)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					yaml, _, err := ResolveArtifact(root, "benchmark", target.endpointID)
					if err != nil {
						b.Fatal(err)
					}
					benchmarkArtifactYAML = yaml
				}
			})
		}
	}
}

func writeBenchmarkArtifact(b testing.TB, path string, content []byte) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
}
