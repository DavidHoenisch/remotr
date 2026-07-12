package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkEngineResult *engine.Engine
var benchmarkDriftReport engine.DriftReport

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
