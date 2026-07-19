//go:build ignore

// Command performance-budget-lint validates the versioned assurance policy.
package main

import (
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/performance-budget-lint.go <budgets.json>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	budgets, err := performance.ParseBudgets(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("performance budgets valid: schema=%d approved=%s metrics=%d mutation=%s\n", budgets.SchemaVersion, budgets.ApprovedAt, len(budgets.Metrics), budgets.Mutation.Tool)
}
