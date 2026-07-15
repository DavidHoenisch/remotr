package resourceregistry_test

import (
	"bytes"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func FuzzCanonicalArtifactRoundTrip(f *testing.F) {
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: package\n        name: curl\n        present: true\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: reboot\n        name: maintenance\n        generation: g1\n        timeout: 15m\n"))
	f.Add([]byte("configurations:\n  - name: base\n    files:\n      - name: motd\n        path: /etc/motd\n        content: managed\n"))
	f.Add([]byte("schemaVersion: 2\nconfigurations: []\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: security\n    resources:\n      - kind: certificate\n        name: service\n        certificatePath: /etc/service/tls.crt\n        privateKeyPath: /etc/service/tls.key\n        certificateRef: remotr:certificates/service@active\n        privateKeyRef: remotr:private-keys/service@7\n        renewalPolicy: provider\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: security\n    resources:\n      - kind: trustAnchor\n        name: corporate-root\n        anchorRef: remotr:trust-anchors/corporate-root@active\n        fingerprint: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: security\n    targetDistros: [Ubuntu]\n    resources:\n      - kind: appArmorProfile\n        name: service\n        profile: usr.bin.service\n        mode: enforce\n        content: 'profile usr.bin.service {}'\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: security\n    resources:\n      - kind: auditRules\n        name: identity\n        rules: ['-w /etc/passwd -p wa -k identity']\n"))

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			return
		}
		state, err := models.ParseState(bytes.NewReader(input))
		if err != nil {
			return
		}
		canonical, err := resourceregistry.MarshalCanonical(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := models.ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical artifact did not parse: %v\n%s", err, canonical)
		}
		recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, recanonical) {
			t.Fatalf("canonical artifact changed after round trip\n--- first ---\n%s--- second ---\n%s", canonical, recanonical)
		}
	})
}
