package main

import (
	"testing"
)

func TestParseAgentStateUsage(t *testing.T) {
	temporary, rollback, err := parseAgentStateUsage([]byte("temporary=4\nrollback=6\n"))
	if err != nil {
		t.Fatal(err)
	}
	if temporary != 4 || rollback != 6 {
		t.Fatalf("temporary=%d rollback=%d, want 4 and 6", temporary, rollback)
	}
}

func TestParseProcessCPUJiffiesHandlesParenthesizedCommand(t *testing.T) {
	stat := []byte("1 (remotr server) S 0 0 0 0 0 0 0 0 0 0 120 30 0 0 0 0 0\n")
	got, err := parseProcessCPUJiffies(stat)
	if err != nil {
		t.Fatal(err)
	}
	if got != 150 {
		t.Fatalf("CPU jiffies=%d, want 150", got)
	}
}
