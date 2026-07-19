package capabilitydoc

import (
	"errors"
	"testing"
)

// OS-AEC-088: the digest is over one deterministic canonical body and a
// submitted mismatch is rejected rather than trusted.
func TestCanonicalDigestMismatchIsRejected(t *testing.T) {
	document := Document{
		DocumentVersion:        1,
		ArtifactSchemaVersions: []int{1, 0},
		Capabilities: []Capability{
			{ID: "resource:packages", Revision: "1"},
			{ID: "provider:init/systemd", Revision: "1.0"},
		},
		Facts: []Fact{
			{Key: "init", Value: "systemd"},
			{Key: "architecture", Value: "x86"},
		},
		AgentVersion: "v1.2.3",
		Digest:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}

	canonical, err := document.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	wantBody := `{"documentVersion":1,"artifactSchemaVersions":[0,1],"capabilities":[{"id":"provider:init/systemd","revision":"1.0"},{"id":"resource:packages","revision":"1"}],"facts":[{"key":"architecture","value":"x86"},{"key":"init","value":"systemd"}],"agentVersion":"v1.2.3"}`
	if string(canonical) != wantBody {
		t.Fatalf("canonical body = %s", canonical)
	}

	digest, err := document.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:6d89ec5fd76142153df0f2f95e94eaeb13f81c7c2f851236c220e4330272fa5e"
	if digest != wantDigest {
		t.Fatalf("digest = %q, want %q", digest, wantDigest)
	}
	if err := document.Validate(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("Validate() error = %v, want ErrDigestMismatch", err)
	}
}
