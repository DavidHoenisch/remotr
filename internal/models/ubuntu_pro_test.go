package models_test

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func TestParseStateUbuntuProBoundaries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fields string
		want   string
	}{
		"invalid lifecycle":       {fields: "lifecycle: present", want: "ubuntuPro lifecycle"},
		"invalid state":           {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: esm-infra, state: pending}]", want: "state must be enabled or disabled"},
		"alias confusion":         {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: cis, state: enabled}]", want: "historical service name"},
		"unsupported mode":        {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: livepatch, state: enabled, enableMode: access-only}]", want: "does not support enableMode"},
		"cross-service variant":   {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: esm-infra, state: enabled, variant: raspi}]", want: "does not support variant"},
		"duplicate field":         {fields: "lifecycle: attached\nlifecycle: detached\ntokenRef: remotr:ubuntu-pro/token@active", want: "mapping key \"lifecycle\" already defined"},
		"removed Landscape block": {fields: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nlandscape: {state: enrolled}", want: "field landscape not found"},
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

// OS-UPM-058: client settings and one-shot maintenance are separate typed
// capabilities, never fields of persistent Ubuntu Pro subscription state.
func TestParseStateUbuntuProRejectsSeparateCapabilitiesWithGuidance(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"APT News":               "aptNews: disabled",
		"proxy":                  "proxy: https://proxy.example.test",
		"refresh timer":          "refresh: 1h",
		"telemetry":              "telemetry: disabled",
		"security fix":           "fix: [CVE-2099-0001]",
		"package upgrade":        "upgradePolicy: security",
		"hardening execution":    "hardening: cis-level-1-server",
		"unattended upgrades":    "unattendedUpgrades: enabled",
		"reboot execution":       "reboot: immediate",
		"contract refresh event": "contractRefresh: now",
	}
	for name, field := range tests {
		name, field := name, field
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := models.ParseState(strings.NewReader(ubuntuProArtifact("lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\n" + field)))
			if err == nil || !strings.Contains(err.Error(), "outside subscription and service lifecycle management") {
				t.Fatalf("ParseState() error = %v, want separate-capability guidance", err)
			}
		})
	}
}

// OS-UPM-052: mutually exclusive boot-impacting services are rejected by
// configuration validation before a provider can perform native discovery or
// mutation.
func TestParseStateUbuntuProRejectsImpossibleBootServicePairs(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"livepatch", "fips"},
		{"livepatch", "fips-updates"},
		{"livepatch", "realtime-kernel"},
		{"fips", "fips-updates"},
		{"fips", "realtime-kernel"},
		{"fips-updates", "realtime-kernel"},
	}
	for _, pair := range pairs {
		pair := pair
		t.Run(pair[0]+" with "+pair[1], func(t *testing.T) {
			t.Parallel()
			fields := fmt.Sprintf("lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices: [{name: %s, state: enabled}, {name: %s, state: enabled}]", pair[0], pair[1])
			_, err := models.ParseState(strings.NewReader(ubuntuProArtifact(fields)))
			if err == nil || !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) || !strings.Contains(err.Error(), "incompatible") {
				t.Fatalf("ParseState() error = %v, want %s/%s incompatibility", err, pair[0], pair[1])
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

func FuzzParseCanonicalUbuntuPro(f *testing.F) {
	f.Add("attached", "esm-infra", "enabled", "full", "", "", "remotr:ubuntu-pro/token@active", uint8(0))
	f.Add("detached", "cis", "disabled", "access-only", "raspi", "purge", "inline:token", uint8(3))
	f.Add("attached", "realtime-kernel", "disabled", "", "intel-iotg", "retain-packages", "remotr:ubuntu-pro/token@7", uint8(4))

	f.Fuzz(func(t *testing.T, lifecycle, service, state, enableMode, variant, disableMode, tokenRef string, flags uint8) {
		values := []string{lifecycle, service, state, enableMode, variant, disableMode, tokenRef}
		for _, value := range values {
			if len(value) > 512 {
				return
			}
		}
		var fields strings.Builder
		fmt.Fprintf(&fields, "lifecycle: %s\ntokenRef: %s\nservices: [{name: %s, state: %s, enableMode: %s, variant: %s, disableMode: %s}]\n",
			strconv.Quote(lifecycle), strconv.Quote(tokenRef), strconv.Quote(service), strconv.Quote(state), strconv.Quote(enableMode), strconv.Quote(variant), strconv.Quote(disableMode))
		if flags&1 != 0 {
			fields.WriteString("args: [enable]\n")
		}
		if flags&2 != 0 {
			fmt.Fprintf(&fields, "lifecycle: %s\n", strconv.Quote(lifecycle))
		}

		parsed, err := models.ParseState(strings.NewReader(ubuntuProArtifact(fields.String())))
		if err != nil {
			if len(err.Error()) > 1024 {
				t.Fatalf("Ubuntu Pro parser diagnostic is unbounded: %d bytes", len(err.Error()))
			}
			return
		}
		canonical, err := resourceregistry.MarshalCanonical(parsed)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := models.ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical Ubuntu Pro state did not parse: %v", err)
		}
		recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, recanonical) {
			t.Fatalf("canonical Ubuntu Pro state changed:\n%s\n%s", canonical, recanonical)
		}
	})
}
