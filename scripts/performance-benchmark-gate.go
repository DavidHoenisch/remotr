//go:build ignore

// Command performance-benchmark-gate evaluates controlled Go benchmark output.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/performance-benchmark-gate.go <budgets.json> <benchmarks.txt>")
		os.Exit(2)
	}
	budgetData, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	budgets, err := performance.ParseBudgets(budgetData)
	if err != nil {
		fail(err)
	}
	benchmarks, err := os.Open(os.Args[2])
	if err != nil {
		fail(err)
	}
	defer benchmarks.Close()
	report, err := performance.EvaluateBenchmarkBudgets(benchmarks, budgets)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fail(err)
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "performance benchmark gate:", err)
	os.Exit(1)
}
