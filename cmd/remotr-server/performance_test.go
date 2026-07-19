package main

import "testing"

func TestPerformanceDiagnosticsAreOptInAndLoopbackOnly(t *testing.T) {
	address, err := performanceDiagnosticsAddress(func(string) string { return "" })
	if err != nil || address != "" {
		t.Fatalf("disabled diagnostics = %q, %v", address, err)
	}
	if _, err := performanceDiagnosticsAddress(func(string) string { return "0.0.0.0:6060" }); err == nil {
		t.Fatal("remote diagnostics address was accepted")
	}
	address, err = performanceDiagnosticsAddress(func(string) string { return "127.0.0.1:6060" })
	if err != nil || address != "127.0.0.1:6060" {
		t.Fatalf("loopback diagnostics = %q, %v", address, err)
	}
}
