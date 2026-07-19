//go:build ignore

// Command performance-relative-gate checks paired Go benchmark collections.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func main() {
	controlled := flag.Bool("controlled", false, "also enforce the controlled-runner latency threshold")
	budgetsPath := flag.String("budgets", "test/performance/budgets.json", "approved budget file")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/performance-relative-gate.go [--controlled] <before.txt> <after.txt>")
		os.Exit(2)
	}
	data, err := os.ReadFile(*budgetsPath)
	if err != nil {
		fail(err)
	}
	budgets, err := performance.ParseBudgets(data)
	if err != nil {
		fail(err)
	}
	before, err := os.Open(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	defer before.Close()
	after, err := os.Open(flag.Arg(1))
	if err != nil {
		fail(err)
	}
	defer after.Close()
	report, err := performance.EvaluateRelativeBenchmarks(before, after, budgets, *controlled)
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
	fmt.Fprintln(os.Stderr, "performance relative gate:", err)
	os.Exit(1)
}
