package evidenceexceptions_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/evidenceexceptions"
)

// Task 11.3: the repository exception inventory is accepted only through its
// explicit review metadata and record-level expiry checks.
func TestRepositoryRegistryIsReviewedAndExpiring(t *testing.T) {
	asOf := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	registry, err := evidenceexceptions.Load(
		filepath.Join("..", "..", "test", "evidence-exceptions.yaml"),
		asOf,
	)
	if err != nil {
		t.Fatalf("Load(repository registry) error = %v", err)
	}
	if registry.Review.ReviewedBy == "" || registry.Review.Scope == "" {
		t.Fatalf("repository registry lacks explicit review metadata: %+v", registry.Review)
	}
	if len(registry.Records) == 0 {
		t.Fatal("repository registry has no reviewed records")
	}
}
