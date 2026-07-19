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
			{Key: "architecture", Value: "amd64"},
		},
		AgentVersion: "v1.2.3",
		Digest:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}

	canonical, err := document.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	wantBody := `{"documentVersion":1,"artifactSchemaVersions":[0,1],"capabilities":[{"id":"provider:init/systemd","revision":"1.0"},{"id":"resource:packages","revision":"1"}],"facts":[{"key":"architecture","value":"amd64"},{"key":"init","value":"systemd"}],"agentVersion":"v1.2.3"}`
	if string(canonical) != wantBody {
		t.Fatalf("canonical body = %s", canonical)
	}

	digest, err := document.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:e37c7f0c39dd99b58ef9c2baec40e46133e2b85a26d8b73651bf570abf6696bd"
	if digest != wantDigest {
		t.Fatalf("digest = %q, want %q", digest, wantDigest)
	}
	if err := document.Validate(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("Validate() error = %v, want ErrDigestMismatch", err)
	}
}
