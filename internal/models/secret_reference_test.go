package models

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseStateAcceptsOnlyReferenceSyntaxForSecretValuedFields(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		valid    bool
	}{
		{
			name:  "remotr repository credential",
			valid: true,
			resource: `kind: aptRepository
        name: private
        url: https://packages.example.test/debian
        suites: [stable]
        components: [main]
        signingKey: vendor
        credentialRef: remotr:repositories/private@active`,
		},
		{
			name: "Remotr selector omitted",
			resource: `kind: aptRepository
        name: private
        url: https://packages.example.test/debian
        suites: [stable]
        components: [main]
        signingKey: vendor
        credentialRef: remotr:repositories/private`,
		},
		{
			name:  "local file password hash",
			valid: true,
			resource: `kind: user
        name: alice
        username: alice
        present: true
        passwordHashRef: local-file:/run/remotr/secrets/alice-password-hash`,
		},
		{
			name: "inline private key",
			resource: `kind: endpointSchedule
        name: rotate
        backend: cron
        schedule: '0 2 * * *'
        user: root
        argv: [/usr/bin/true]
        environment:
          - name: PRIVATE_KEY
            secretRef: '-----BEGIN PRIVATE KEY-----'`,
		},
		{
			name: "inline password",
			resource: `kind: user
        name: alice
        username: alice
        present: true
        passwordHashRef: inline:hunter2`,
		},
		{
			name: "relative local file",
			resource: `kind: networkProfile
        name: wifi
        provider: network-manager
        selector: {name: wlan0}
        profileName: office
        profileType: wifi
        ssid: corp
        credentialRef: local-file:secrets/wifi`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := "schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - " + test.resource + "\n"
			_, err := ParseState(strings.NewReader(input))
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && (err == nil || !strings.Contains(err.Error(), "secret reference")) {
				t.Fatalf("error = %v, want secret reference rejection", err)
			}
		})
	}
}

func FuzzParseStateNeverTreatsInlinePrivateKeyAsSecretReference(f *testing.F) {
	for _, value := range []string{"-----BEGIN PRIVATE KEY-----", "inline:password", "local-file:relative"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 512 {
			t.Skip()
		}
		input := fmt.Sprintf("schemaVersion: 1\nconfigurations:\n- name: base\n  resources:\n  - kind: endpointSchedule\n    name: rotate\n    backend: cron\n    schedule: '0 2 * * *'\n    user: root\n    argv: [/usr/bin/true]\n    environment:\n    - name: PRIVATE_KEY\n      secretRef: %q\n", value)
		_, err := ParseState(strings.NewReader(input))
		if strings.Contains(value, "PRIVATE KEY") && err == nil {
			t.Fatal("inline private-key material was accepted as a reference")
		}
	})
}
