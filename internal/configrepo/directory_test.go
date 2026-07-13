package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-FOM-008: authoring must reject a directory path that cannot be traversed
// safely before it can reach an endpoint.
func TestValidateStateRejectsRelativeDirectoryPath(t *testing.T) {
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Directories: []models.DirectoryResource{{
			Name:         "unsafe",
			Path:         "relative/path",
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "absolute non-root path") {
		t.Fatalf("ValidateState() error = %v, want absolute-path rejection", err)
	}
}

// OS-FOM-007: a present link without a target is not actionable desired
// state and must be rejected at the configuration authoring seam.
func TestValidateStateRejectsPresentLinkWithoutTarget(t *testing.T) {
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Links: []models.LinkResource{{
			Name:     "current",
			Path:     "/opt/example/current",
			LinkType: models.LinkTypeSymbolic,
			ResourceMeta: models.ResourceMeta{
				Lifecycle: models.LifecyclePresent,
			},
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("ValidateState() error = %v, want missing-target rejection", err)
	}
}

// OS-FOM-009: only an explicitly authoritative directory may purge an owned
// recursive tree.
func TestValidateStateRejectsNonAuthoritativeDirectoryPurge(t *testing.T) {
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Directories: []models.DirectoryResource{{
			Name:       "managed",
			Path:       "/var/lib/example",
			Recursive:  true,
			Purge:      true,
			MaxDepth:   2,
			MaxEntries: 10,
			ResourceMeta: models.ResourceMeta{
				Lifecycle: models.LifecyclePresent,
				Ownership: models.OwnershipNamed,
			},
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "requires authoritative ownership") {
		t.Fatalf("ValidateState() error = %v, want ownership rejection", err)
	}
}

// OS-LIA-001: system-group intent must carry a fixed GID so it can be checked
// and reassigned without guessing a distribution-specific allocation range.
func TestValidateStateRejectsSystemGroupWithoutFixedGID(t *testing.T) {
	system := true
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Groups: []models.GroupResource{{
			Name:   "operators",
			Group:  "operators",
			System: &system,
			ResourceMeta: models.ResourceMeta{
				Lifecycle: models.LifecyclePresent,
			},
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "system class requires a fixed gid") {
		t.Fatalf("ValidateState() error = %v, want system-GID rejection", err)
	}
}

// OS-LIA-004: supplementary membership has no safe implicit ownership mode.
func TestValidateStateRejectsSupplementaryGroupsWithoutMode(t *testing.T) {
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Users: []models.UserResource{{
			Name: "alice", Username: "alice", Present: true,
			SupplementaryGroups: []string{"docker"},
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "requires supplementaryGroupsMode") {
		t.Fatalf("ValidateState() error = %v, want membership-mode rejection", err)
	}
}

// OS-LIA-006: authored configuration may name a password source but never
// contain password-hash material directly.
func TestValidateStateRejectsInlinePasswordHash(t *testing.T) {
	err := ValidateState(models.State{Configurations: []models.Configuration{{
		Name: "base",
		Users: []models.UserResource{{
			Name: "alice", Username: "alice", Present: true, PasswordHashRef: "$6$inline$hash",
		}},
	}}}, "test")
	if err == nil || !strings.Contains(err.Error(), "file:/ secret reference") {
		t.Fatalf("ValidateState() error = %v, want password-reference rejection", err)
	}
}
