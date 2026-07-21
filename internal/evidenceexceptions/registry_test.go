package evidenceexceptions_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/evidenceexceptions"
	"gopkg.in/yaml.v3"
)

const validRegistry = `version: 1
review:
  reviewed_by: "@reviewer"
  reviewed_at: 2026-07-20
  scope: openspec:example#1
records:
- id: EXC-999
  kind: manual
  owner: "@owner"
  issue: openspec:example#1
  reason: External fixture must be supplied by its isolated target.
  expires: 2026-07-21
`

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

func TestDecodeRejectsUnreviewedOrNonExpiringRecords(t *testing.T) {
	asOf := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unknown version", raw: strings.Replace(validRegistry, "version: 1", "version: 2", 1), want: "want 1"},
		{name: "future review", raw: strings.Replace(validRegistry, "reviewed_at: 2026-07-20", "reviewed_at: 2026-07-21", 1), want: "after validation date"},
		{name: "missing review scope", raw: strings.Replace(validRegistry, "scope: openspec:example#1", `scope: ""`, 1), want: "requires reviewed_by and scope"},
		{name: "unknown kind", raw: strings.Replace(validRegistry, "kind: manual", "kind: deferred", 1), want: "kind \"deferred\" is not supported"},
		{name: "placeholder issue", raw: strings.Replace(validRegistry, "issue: openspec:example#1", "issue: pending-triage", 1), want: "reviewed issue"},
		{name: "expiry boundary", raw: strings.Replace(validRegistry, "expires: 2026-07-21", "expires: 2026-07-20", 1), want: "expired on 2026-07-20"},
		{name: "equivalent mutant lacks selector", raw: strings.Replace(validRegistry, "kind: manual", "kind: equivalent-mutant", 1), want: "requires equivalent_selector"},
		{name: "unknown field", raw: validRegistry + "unknown: true\n", want: "field unknown not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := evidenceexceptions.Decode([]byte(test.raw), asOf); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want %q", err, test.want)
			}
		})
	}

	if _, err := evidenceexceptions.Decode([]byte(validRegistry), asOf); err != nil {
		t.Fatalf("Decode(expiry tomorrow) error = %v", err)
	}
}

func FuzzDecodeRegistryRoundTrip(f *testing.F) {
	asOf := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	f.Add([]byte(validRegistry))
	f.Add([]byte("version: 1\nrecords: []\n"))
	f.Add([]byte{0, 0xff, '\n'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			return
		}
		registry, err := evidenceexceptions.Decode(raw, asOf)
		if err != nil {
			return
		}
		encoded, err := yaml.Marshal(registry)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := evidenceexceptions.Decode(encoded, asOf)
		if err != nil {
			t.Fatalf("accepted registry failed round trip: %v\n%s", err, encoded)
		}
		if !reflect.DeepEqual(roundTripped, registry) {
			t.Fatalf("registry round trip changed value:\nfirst:  %+v\nsecond: %+v", registry, roundTripped)
		}
	})
}
