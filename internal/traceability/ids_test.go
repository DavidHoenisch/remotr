package traceability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerificationIDFromComment(t *testing.T) {
	registry, err := LoadPrefixRegistry(filepath.Join("..", "..", "test", "traceability-prefixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	id, err := VerificationIDFromComment("<!-- verification-id: OS-AEC-001 -->", registry)
	if err != nil {
		t.Fatal(err)
	}
	if id.Prefix != "OS-AEC" || id.Sequence != 1 {
		t.Fatalf("parsed ID = %#v", id)
	}
}

func TestVerificationIDRejectsMalformedOrUnregisteredValues(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{
		"OS-AEC": {Change: "change", Capability: "capability"},
	}}
	for _, value := range []string{
		"<!-- verification-id: OS-AEC-01 -->",
		"<!-- verification-id: OS-UNKNOWN-001 -->",
		"verification-id: OS-AEC-001",
	} {
		if _, err := VerificationIDFromComment(value, registry); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}

func TestLoadPrefixRegistryRejectsInvalidOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefixes.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nprefixes:\n  bad: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPrefixRegistry(path); err == nil {
		t.Fatal("expected invalid registry to fail")
	}
}
