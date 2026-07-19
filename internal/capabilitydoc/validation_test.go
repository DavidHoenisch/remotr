package capabilitydoc

import (
	"errors"
	"strings"
	"testing"
)

// OS-AEC-089: malformed and unbounded documents fail closed with bounded
// diagnostics before they can be persisted or used for selection.
func TestValidateRejectsMalformedOrUnboundedDocuments(t *testing.T) {
	valid := func() Document {
		document, err := (Document{
			DocumentVersion:        1,
			ArtifactSchemaVersions: []int{0, 1},
			Capabilities: []Capability{
				{ID: "provider:init/systemd", Revision: "1"},
				{ID: "resource:package", Revision: "package-v1"},
			},
			Facts: []Fact{
				{Key: "architecture", Value: "x86"},
				{Key: "desktop", Value: "dconf"},
			},
			AgentVersion: "v1.2.3",
		}).WithCanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}
		return document
	}
	signed := func(mutate func(*Document)) Document {
		document := valid()
		mutate(&document)
		signed, err := document.WithCanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}

	tests := []struct {
		name string
		doc  Document
		code string
	}{
		{"unsupported document version", signed(func(d *Document) { d.DocumentVersion = 2 }), "unsupported_document_version"},
		{"no artifact schemas", signed(func(d *Document) { d.ArtifactSchemaVersions = nil }), "artifact_schema_count"},
		{"duplicate artifact schema", signed(func(d *Document) { d.ArtifactSchemaVersions = []int{1, 1} }), "duplicate_artifact_schema"},
		{"impossible artifact schema", signed(func(d *Document) { d.ArtifactSchemaVersions = []int{99} }), "unsupported_artifact_schema"},
		{"too many capabilities", signed(func(d *Document) { d.Capabilities = make([]Capability, MaxCapabilities+1) }), "capability_count"},
		{"duplicate capability", signed(func(d *Document) { d.Capabilities = append(d.Capabilities, d.Capabilities[0]) }), "duplicate_capability"},
		{"conflicting capability", signed(func(d *Document) {
			d.Capabilities = append(d.Capabilities, Capability{ID: d.Capabilities[0].ID, Revision: "2"})
		}), "conflicting_capability_revision"},
		{"malformed capability id", signed(func(d *Document) { d.Capabilities[0].ID = "Provider Secret" }), "capability_id"},
		{"oversized capability id", signed(func(d *Document) { d.Capabilities[0].ID = "resource:" + strings.Repeat("a", MaxCapabilityIDBytes) }), "capability_id"},
		{"malformed revision", signed(func(d *Document) { d.Capabilities[0].Revision = "1 secret" }), "contract_revision"},
		{"too many facts", signed(func(d *Document) { d.Facts = make([]Fact, MaxFacts+1) }), "fact_count"},
		{"duplicate fact", signed(func(d *Document) { d.Facts = append(d.Facts, d.Facts[0]) }), "duplicate_fact"},
		{"conflicting scalar fact", signed(func(d *Document) { d.Facts = append(d.Facts, Fact{Key: "architecture", Value: "arm"}) }), "conflicting_fact"},
		{"unknown fact", signed(func(d *Document) { d.Facts[0].Key = "credential" }), "fact_key"},
		{"malformed fact value", signed(func(d *Document) { d.Facts[0].Value = "SECRET value" }), "fact_value"},
		{"oversized agent version", signed(func(d *Document) { d.AgentVersion = strings.Repeat("v", MaxAgentVersionBytes+1) }), "agent_version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.doc.Validate()
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != test.code {
				t.Fatalf("Validate() error = %v, want code %q", err, test.code)
			}
			if len(err.Error()) > MaxDiagnosticBytes {
				t.Fatalf("diagnostic is unbounded: %d bytes", len(err.Error()))
			}
		})
	}
}

func TestDecodeRejectsOversizeUnknownAndTrailingJSON(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  []byte
		code string
	}{
		{"oversize", []byte(strings.Repeat(" ", MaxDocumentBytes+1)), "document_size"},
		{"unknown field", []byte(`{"documentVersion":1,"unexpected":true}`), "document_json"},
		{"trailing value", []byte(`{} {}`), "document_json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(test.raw)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != test.code {
				t.Fatalf("Decode() error = %v, want code %q", err, test.code)
			}
		})
	}
}
