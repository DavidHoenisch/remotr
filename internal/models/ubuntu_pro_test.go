package models_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func TestParseStateUbuntuProBoundaries(t *testing.T) {
	t.Parallel()

	tags := make([]string, 33)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	tests := map[string]struct {
		fields string
		want   string
	}{
		"invalid lifecycle":     {fields: "lifecycle: present", want: "ubuntuPro lifecycle"},
		"invalid state":         {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: esm-infra, state: pending}]", want: "state must be enabled or disabled"},
		"alias confusion":       {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: cis, state: enabled}]", want: "historical service name"},
		"unsupported mode":      {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: livepatch, state: enabled, enableMode: access-only}]", want: "does not support enableMode"},
		"cross-service variant": {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: esm-infra, state: enabled, variant: raspi}]", want: "does not support variant"},
		"duplicate field":       {fields: "lifecycle: attached\nlifecycle: detached\ntokenRef: remotr:ubuntu-pro/token@active", want: "mapping key \"lifecycle\" already defined"},
		"oversized tags":        {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nlandscape: {state: enrolled, accountName: production, computerTitle: host, tags: [" + strings.Join(tags, ", ") + "]}", want: "tags exceeds 32 entries"},
		"oversized title":       {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nlandscape: {state: enrolled, accountName: production, computerTitle: " + strings.Repeat("x", 257) + "}", want: "computerTitle exceeds 256 bytes"},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := models.ParseState(strings.NewReader(ubuntuProArtifact(test.fields)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseState() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseStateUbuntuProCanonicalRoundTripIsDeterministic(t *testing.T) {
	t.Parallel()

	state, err := models.ParseState(strings.NewReader(ubuntuProArtifact("lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: realtime-kernel, state: disabled, variant: raspi, disableMode: purge}]")))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := resourceregistry.MarshalCanonical(state)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped, err := models.ParseState(bytes.NewReader(canonical))
	if err != nil {
		t.Fatal(err)
	}
	recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, recanonical) {
		t.Fatalf("Ubuntu Pro canonical form changed:\nfirst:\n%s\nsecond:\n%s", canonical, recanonical)
	}
}

func ubuntuProArtifact(fields string) string {
	return "schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: ubuntuPro\n        name: subscription\n        " + strings.ReplaceAll(fields, "\n", "\n        ") + "\n"
}
