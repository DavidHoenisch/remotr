package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalTrustAnchorRequiresFingerprintAndExplicitReference(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    resources:
      - kind: trustAnchor
        name: corporate-root
        anchorRef: remotr:trust-anchors/corporate-root@7
        fingerprint: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].TrustAnchors) != 1 || state.Configurations[0].TrustAnchors[0].Kind != models.ResourceKindTrustAnchor {
		t.Fatalf("parsed state = %#v", state)
	}

	_, err = models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    resources:
      - kind: trustAnchor
        name: corporate-root
        anchorRef: remotr:trust-anchors/corporate-root
        fingerprint: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`))
	if err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("omitted selector error = %v", err)
	}
}
