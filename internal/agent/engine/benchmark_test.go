package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkEngineResult *engine.Engine
var benchmarkDriftReport engine.DriftReport
var benchmarkAgentCycleReport []byte

func BenchmarkEngineBuildDependencyOrder(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		state := benchmarkResolvedState(size)
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				built, err := engine.New(state, benchmarkFacts(), benchmarkRunner{}, nil)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkEngineResult = built
			}
		})
	}
}

func BenchmarkEngineCheckAllReport(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		built, err := engine.New(benchmarkResolvedState(size), benchmarkFacts(), benchmarkRunner{}, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkDriftReport = built.CheckAll(ctx)
			}
		})
	}
}

func BenchmarkAgentFullCycleCompliant(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		raw := benchmarkCommandArtifact(size)
		b.Run("resources="+size.String(), func(b *testing.B) {
			benchmarkAgentFullCycle(b, raw, size, true)
		})
	}
}

func BenchmarkAgentFullCycleDriftedApply(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		raw := benchmarkCommandArtifact(size)
		b.Run("resources="+size.String(), func(b *testing.B) {
			benchmarkAgentFullCycle(b, raw, size, false)
		})
	}
}

func benchmarkAgentFullCycle(b *testing.B, raw []byte, size benchmarkfixture.ResourceCount, compliant bool) {
	b.Helper()
	stateDir := b.TempDir()
	before := benchmarkProcessSnapshot()
	var reportBytes int
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state, err := models.ParseState(bytes.NewReader(raw))
		if err != nil {
			b.Fatal(err)
		}
		resolved := resolve.Resolve(state, benchmarkFacts())
		built, err := engine.New(resolved, benchmarkFacts(), benchmarkCycleRunner{compliant: compliant}, nil, engine.WithStateDir(stateDir))
		if err != nil {
			b.Fatal(err)
		}
		drift := built.CheckAll(context.Background())
		if drift.InCompliance != compliant || len(drift.Items) != int(size) {
			b.Fatalf("compliance=%t items=%d, want compliance=%t items=%d", drift.InCompliance, len(drift.Items), compliant, size)
		}
		result := engine.ApplyResult{}
		if !compliant {
			result = built.ApplyAll(context.Background(), engine.PolicyAuto)
			if result.Failed != nil || len(result.Applied) != int(size) {
				b.Fatalf("apply result=%+v, want %d applied resources", result, size)
			}
		}
		encoded, err := json.Marshal(struct {
			Drift engine.DriftReport `json:"drift"`
			Apply engine.ApplyResult `json:"apply"`
		}{Drift: drift, Apply: result})
		if err != nil {
			b.Fatal(err)
		}
		benchmarkAgentCycleReport = encoded
		reportBytes = len(encoded)
	}
	b.StopTimer()
	after := benchmarkProcessSnapshot()
	rollbackBytes, err := benchmarkDirectoryBytes(stateDir)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(after.cpu-before.cpu)/float64(b.N), "cpu-ns/op")
	b.ReportMetric(float64(after.peakRSSBytes), "peak-RSS-bytes")
	b.ReportMetric(float64(max(before.goroutines, after.goroutines)), "goroutines")
	b.ReportMetric(float64(after.readBlocks-before.readBlocks)/float64(b.N), "disk-read-blocks/op")
	b.ReportMetric(float64(after.writeBlocks-before.writeBlocks)/float64(b.N), "disk-write-blocks/op")
	b.ReportMetric(float64(len(raw)), "artifact-bytes/op")
	b.ReportMetric(float64(reportBytes), "report-bytes/op")
	b.ReportMetric(float64(rollbackBytes), "rollback-storage-bytes")
}

func benchmarkCommandArtifact(size benchmarkfixture.ResourceCount) []byte {
	var out strings.Builder
	out.Grow(64 + int(size)*112)
	out.WriteString("configurations:\n  - name: benchmark\n    commands:\n")
	for i := 0; i < int(size); i++ {
		fmt.Fprintf(&out, "      - name: command-%04d\n        risk: normal\n        check: [check, command-%04d]\n        apply: [apply, command-%04d]\n", i, i, i)
	}
	return []byte(out.String())
}

type benchmarkCycleRunner struct{ compliant bool }

func (r benchmarkCycleRunner) Run(name string, _ ...string) ([]byte, []byte, error) {
	if name == "check" && !r.compliant {
		return nil, nil, errors.New("benchmark drift")
	}
	return nil, nil, nil
}

type benchmarkResourceSnapshot struct {
	cpu          int64
	peakRSSBytes uint64
	readBlocks   int64
	writeBlocks  int64
	goroutines   int
}

func benchmarkProcessSnapshot() benchmarkResourceSnapshot {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	return benchmarkResourceSnapshot{
		cpu:          timevalDuration(usage.Utime) + timevalDuration(usage.Stime),
		peakRSSBytes: uint64(usage.Maxrss) * 1024,
		readBlocks:   usage.Inblock,
		writeBlocks:  usage.Oublock,
		goroutines:   runtime.NumGoroutine(),
	}
}

func timevalDuration(value syscall.Timeval) int64 {
	return value.Sec*int64(time.Second) + value.Usec*int64(time.Microsecond)
}

func benchmarkDirectoryBytes(root string) (int64, error) {
	var total int64
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			child, err := benchmarkDirectoryBytes(filepath.Join(root, entry.Name()))
			if err != nil {
				return 0, err
			}
			total += child
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func benchmarkResolvedState(size benchmarkfixture.ResourceCount) resolve.ResolvedState {
	resources := make([]models.CommandResource, 0, size)
	for i := 0; i < int(size); i++ {
		resource := models.CommandResource{Name: fmt.Sprintf("command-%04d", i), Check: []string{"benchmark"}}
		if i > 0 {
			resource.DependsOn = []string{models.ResourceAddress("benchmark", fmt.Sprintf("command-%04d", i-1))}
		}
		resources = append(resources, resource)
	}
	return resolve.ResolvedState{Configurations: []models.Configuration{{Name: "benchmark", Commands: resources}}}
}

func benchmarkFacts() facts.Facts { return facts.Facts{Distro: types.Debian, Arch: types.X86} }

type benchmarkRunner struct{}

func (benchmarkRunner) Run(string, ...string) ([]byte, []byte, error) {
	return nil, nil, errors.New("benchmark drift")
}
