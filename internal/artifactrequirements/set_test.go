package artifactrequirements

import (
	"bytes"
	"testing"
)

func TestRequirementSetCanonicalDigestIsDeterministic(t *testing.T) {
	first := Set{
		Version:               1,
		ArtifactSchemaVersion: 1,
		ResourceCapabilities: []Requirement{
			{ID: "resource:service", Revision: "service-v1"},
			{ID: "resource:package", Revision: "package-v1"},
		},
		ProviderCapabilities: []Requirement{
			{ID: "provider:init/systemd", Revision: "1"},
			{ID: "provider:package/apt", Revision: "1"},
		},
	}
	second := Set{
		Version:               1,
		ArtifactSchemaVersion: 1,
		ResourceCapabilities: []Requirement{
			{ID: "resource:package", Revision: "package-v1"},
			{ID: "resource:service", Revision: "service-v1"},
		},
		ProviderCapabilities: []Requirement{
			{ID: "provider:package/apt", Revision: "1"},
			{ID: "provider:init/systemd", Revision: "1"},
		},
	}

	firstBody, err := first.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := second.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatalf("canonical bodies differ:\n%s\n%s", firstBody, secondBody)
	}
	const wantBody = `{"version":1,"artifactSchemaVersion":1,"resourceCapabilities":[{"id":"resource:package","revision":"package-v1"},{"id":"resource:service","revision":"service-v1"}],"providerCapabilities":[{"id":"provider:init/systemd","revision":"1"},{"id":"provider:package/apt","revision":"1"}]}`
	if string(firstBody) != wantBody {
		t.Fatalf("canonical body = %s", firstBody)
	}
	firstDigest, err := first.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := second.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:71bd5e29f9b4c87f7cc35a4387f3b60351ffc100d8c9e68423e3583e61af53a7"
	if firstDigest != secondDigest || firstDigest != wantDigest {
		t.Fatalf("digests = %q and %q", firstDigest, secondDigest)
	}
}

func TestDecodePersistedAcceptsJSONBNormalizationButRejectsDigestMismatch(t *testing.T) {
	set := Set{
		Version: CurrentVersion, ArtifactSchemaVersion: 1,
		ResourceCapabilities: []Requirement{{ID: "resource:package", Revision: "package-v1"}},
		ProviderCapabilities: []Requirement{{ID: "provider:package/apt", Revision: "1"}},
	}
	digest, err := set.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	jsonbNormalized := []byte(`{"providerCapabilities":[{"revision":"1","id":"provider:package/apt"}],"resourceCapabilities":[{"revision":"package-v1","id":"resource:package"}],"artifactSchemaVersion":1,"version":1}`)
	decoded, err := DecodePersisted(jsonbNormalized, digest)
	if err != nil || decoded.ArtifactSchemaVersion != 1 {
		t.Fatalf("decode JSONB-normalized requirement set = %+v, err=%v", decoded, err)
	}
	if _, err := DecodePersisted(jsonbNormalized, "sha256:0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("JSONB-normalized requirement set bypassed canonical digest verification")
	}
	if _, err := DecodePersisted(append(jsonbNormalized[:len(jsonbNormalized)-1], []byte(`,"unknown":true}`)...), digest); err == nil {
		t.Fatal("JSONB-normalized requirement set accepted an unknown field")
	}
}
