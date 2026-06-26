package main

import (
	"strings"
	"testing"
)

func TestFilterContentLines(t *testing.T) {
	t.Parallel()
	content := strings.Join([]string{
		"kernel: boot ok",
		"remotr-agent: sync unchanged",
		"systemd: started remotr-agent.service",
	}, "\n")

	got := filterContentLines("sync", content)
	if !strings.Contains(got, "remotr-agent: sync unchanged") {
		t.Fatalf("expected sync line, got %q", got)
	}
	if strings.Contains(got, "kernel: boot ok") {
		t.Fatalf("did not expect unrelated line, got %q", got)
	}

	if filterContentLines("", content) != content {
		t.Fatal("empty query should return full content")
	}
	if filterContentLines("missing", content) != "(no matches)" {
		t.Fatal("expected no matches message")
	}
}
