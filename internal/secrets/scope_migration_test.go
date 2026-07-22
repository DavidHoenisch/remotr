package secrets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestLegacyScopeClassificationRejectsAmbiguityWithoutChangingAuthenticatedEnvelope(t *testing.T) {
	keyring, err := NewKeyring("kek-migration", map[string][]byte{"kek-migration": bytes.Repeat([]byte{0x91}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := envelope.Encrypt(ScopeMetadata{Name: "legacy/fleet", Version: "1", Fleet: "production"}, []byte("migration-canary"))
	if err != nil {
		t.Fatal(err)
	}
	original := legacy.Clone()
	kind, fleet, endpointID, err := normalizeScope(legacy.Scope.Scope, legacy.Scope.Fleet, legacy.Scope.EndpointID, true)
	if err != nil || kind != ScopeFleet || fleet != "production" || endpointID != "" {
		t.Fatalf("legacy scope = %q, %q, %q, %v", kind, fleet, endpointID, err)
	}
	if !reflect.DeepEqual(legacy, original) {
		t.Fatal("legacy scope classification changed authenticated envelope metadata")
	}
	plaintext, err := envelope.Decrypt(legacy)
	if err != nil || string(plaintext) != "migration-canary" {
		t.Fatalf("legacy decrypt = %q, %v", plaintext, err)
	}

	for _, scope := range []ScopeMetadata{
		{Name: "invalid/neither", Version: "1"},
		{Name: "invalid/both", Version: "1", Fleet: "production", EndpointID: "endpoint-1"},
	} {
		if _, _, _, err := normalizeScope(scope.Scope, scope.Fleet, scope.EndpointID, true); err == nil {
			t.Fatalf("ambiguous legacy scope accepted: %#v", scope)
		}
	}
}
