package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LIA-015: configuration validation rejects two sudo resources whose
// effective sudoers.d fragment filenames collide (for example priority: 20
// with name: ops colliding with name: 20-ops).
func TestValidateState_RejectsCollidingSudoFragmentFilenames(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: access
    resources:
      - kind: sudo
        name: ops
        priority: 20
        lifecycle: present
        ownership: fragment
        defaults:
          - option: use_pty
        recoveryPrincipals: [recovery]
      - kind: sudo
        name: 20-ops
        lifecycle: present
        ownership: fragment
        subjects: [developer]
        commands: [/usr/bin/id]
        recoveryPrincipals: [recovery]
`
	state, err := models.ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	err = ValidateState(state, "sudo-collision")
	if err == nil || !strings.Contains(err.Error(), "20-ops") {
		t.Fatalf("ValidateState() error = %v, want fragment filename collision on 20-ops", err)
	}
}

func TestValidateState_AllowsDistinctSudoFragmentFilenames(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: access
    resources:
      - kind: sudo
        name: hardening
        priority: 10
        lifecycle: present
        ownership: fragment
        defaults:
          - option: use_pty
        recoveryPrincipals: [recovery]
      - kind: sudo
        name: ops
        priority: 20
        lifecycle: present
        ownership: fragment
        subjects: [developer]
        commands: [/usr/bin/id]
        recoveryPrincipals: [recovery]
`
	state, err := models.ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if err := ValidateState(state, "sudo-distinct"); err != nil {
		t.Fatalf("ValidateState() error = %v", err)
	}
}
