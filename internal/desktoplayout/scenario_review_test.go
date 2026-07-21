package desktoplayout_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestEveryDesktopScenarioHasPassingReleaseReview(t *testing.T) {
	root := repositoryRoot(t)
	changeRoot := filepath.Join(root, "openspec", "changes", "archive", "2026-07-21-add-desktop-admin-app")
	expected := desktopScenarioIDs(t, changeRoot)
	if len(expected) != 83 {
		t.Fatalf("desktop OpenSpec scenario count = %d, want 83", len(expected))
	}

	type scenarioReview struct {
		ID        string   `json:"id"`
		Result    string   `json:"result"`
		Selectors []string `json:"selectors"`
		Exception string   `json:"exception,omitempty"`
	}
	var review struct {
		SchemaVersion int    `json:"schemaVersion"`
		Reviewed      string `json:"reviewed"`
		Claims        struct {
			ParityClaim            string   `json:"parityClaim"`
			PlannedParity          int      `json:"plannedParity"`
			OperatingSystems       []string `json:"operatingSystems"`
			Architectures          []string `json:"architectures"`
			PackageFormats         []string `json:"packageFormats"`
			Publications           []string `json:"publications"`
			ReleaseEligibleFormats []string `json:"releaseEligibleFormats"`
			SignedOutput           bool     `json:"signedOutput"`
		} `json:"claims"`
		Scenarios []scenarioReview `json:"scenarios"`
	}
	reviewPath := filepath.Join(changeRoot, "evidence", "14.5-scenario-review.json")
	reviewData, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("read desktop scenario release review: %v", err)
	}
	if err := json.Unmarshal(reviewData, &review); err != nil {
		t.Fatalf("parse desktop scenario release review: %v", err)
	}
	if review.SchemaVersion != 1 || review.Reviewed != "2026-07-16" {
		t.Errorf("scenario review identity = version %d/date %q", review.SchemaVersion, review.Reviewed)
	}

	exceptions, err := os.ReadFile(filepath.Join(root, "test", "evidence-exceptions.yaml"))
	if err != nil {
		t.Fatalf("read approved evidence exceptions: %v", err)
	}
	seen := make(map[string]bool, len(review.Scenarios))
	for _, record := range review.Scenarios {
		if seen[record.ID] {
			t.Errorf("desktop scenario review duplicates %s", record.ID)
		}
		seen[record.ID] = true
		if !expected[record.ID] {
			t.Errorf("desktop scenario review contains unknown ID %s", record.ID)
		}
		switch record.Result {
		case "passed":
			if len(record.Selectors) == 0 {
				t.Errorf("desktop scenario %s has no passing selector", record.ID)
			}
			if record.Exception != "" {
				t.Errorf("desktop scenario %s passed but also claims exception %s", record.ID, record.Exception)
			}
		case "exception":
			if record.Exception == "" || !strings.Contains(string(exceptions), "- id: "+record.Exception+"\n") {
				t.Errorf("desktop scenario %s names unapproved exception %q", record.ID, record.Exception)
			}
		default:
			t.Errorf("desktop scenario %s has invalid review result %q", record.ID, record.Result)
		}
		for _, selector := range record.Selectors {
			if !strings.HasPrefix(selector, "make ") &&
				!strings.HasPrefix(selector, "mise exec -- go test ") &&
				!strings.HasPrefix(selector, "cd desktop && go test ") &&
				!strings.HasPrefix(selector, "cd desktop/frontend && pnpm ") {
				t.Errorf("desktop scenario %s has unsupported selector %q", record.ID, selector)
			}
		}
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("desktop scenario release review is missing %s", id)
		}
	}

	assertDesktopReviewClaimsMatchInventories(t, root, review.Claims.ParityClaim, review.Claims.PlannedParity,
		review.Claims.OperatingSystems, review.Claims.Architectures, review.Claims.PackageFormats,
		review.Claims.Publications, review.Claims.ReleaseEligibleFormats, review.Claims.SignedOutput)
}

func desktopScenarioIDs(t *testing.T, changeRoot string) map[string]bool {
	t.Helper()
	pattern := regexp.MustCompile(`verification-id:\s*(OS-(?:DOA|DFV|DFA)-[0-9]{3})`)
	ids := map[string]bool{}
	for _, capability := range []string{"desktop-operator-access", "desktop-fleet-visibility", "desktop-fleet-actions"} {
		path := filepath.Join(changeRoot, "specs", capability, "spec.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s scenarios: %v", capability, err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			if ids[match[1]] {
				t.Fatalf("duplicate desktop scenario ID %s", match[1])
			}
			ids[match[1]] = true
		}
	}
	return ids
}

func assertDesktopReviewClaimsMatchInventories(t *testing.T, root, parityClaim string, plannedParity int, operatingSystems, architectures, packageFormats, publications, releaseEligibleFormats []string, signedOutput bool) {
	t.Helper()
	parityData, err := os.ReadFile(filepath.Join(root, "docs", "reference", "desktop-cli-parity.json"))
	if err != nil {
		t.Fatalf("read desktop parity inventory: %v", err)
	}
	var parity struct {
		Publication struct {
			ParityClaim string `json:"parity_claim"`
		} `json:"publication"`
		Entries []struct {
			Status string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(parityData, &parity); err != nil {
		t.Fatalf("parse desktop parity inventory: %v", err)
	}
	planned := 0
	for _, entry := range parity.Entries {
		if entry.Status == "planned" {
			planned++
		}
	}
	if parityClaim != parity.Publication.ParityClaim || plannedParity != planned || parityClaim != "partial" || planned == 0 {
		t.Errorf("review parity claim/count = %q/%d, live inventory = %q/%d", parityClaim, plannedParity, parity.Publication.ParityClaim, planned)
	}

	targetData, err := os.ReadFile(filepath.Join(root, "desktop", "build", "linux", "package-targets.json"))
	if err != nil {
		t.Fatalf("read desktop package targets: %v", err)
	}
	var targets struct {
		SignedReleaseOutput struct {
			Configured bool `json:"configured"`
		} `json:"signedReleaseOutput"`
		Artifacts []struct {
			OS              string `json:"os"`
			Architecture    string `json:"architecture"`
			PackageFormat   string `json:"packageFormat"`
			Publication     string `json:"publication"`
			ReleaseEligible bool   `json:"releaseEligible"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(targetData, &targets); err != nil {
		t.Fatalf("parse desktop package targets: %v", err)
	}
	if len(targets.Artifacts) != 2 {
		t.Fatalf("live desktop package target count = %d, want DEB and Flatpak", len(targets.Artifacts))
	}
	liveOperatingSystems := make([]string, 0, len(targets.Artifacts))
	liveArchitectures := make([]string, 0, len(targets.Artifacts))
	livePackageFormats := make([]string, 0, len(targets.Artifacts))
	livePublications := make([]string, 0, len(targets.Artifacts))
	liveReleaseEligibleFormats := make([]string, 0, len(targets.Artifacts))
	for _, target := range targets.Artifacts {
		liveOperatingSystems = append(liveOperatingSystems, target.OS)
		liveArchitectures = append(liveArchitectures, target.Architecture)
		livePackageFormats = append(livePackageFormats, target.PackageFormat)
		livePublications = append(livePublications, target.Publication)
		if target.ReleaseEligible {
			liveReleaseEligibleFormats = append(liveReleaseEligibleFormats, target.PackageFormat)
		}
	}
	claimSets := [][]string{operatingSystems, architectures, packageFormats, publications, releaseEligibleFormats}
	liveSets := [][]string{liveOperatingSystems, liveArchitectures, livePackageFormats, livePublications, liveReleaseEligibleFormats}
	labels := []string{"operating systems", "architectures", "package formats", "publications", "release-eligible formats"}
	for index := range claimSets {
		claimed := uniqueSorted(claimSets[index])
		live := uniqueSorted(liveSets[index])
		if strings.Join(claimed, ",") != strings.Join(live, ",") {
			t.Errorf("review %s = %v, live = %v", labels[index], claimed, live)
		}
	}
	if signedOutput != targets.SignedReleaseOutput.Configured {
		t.Errorf("review signed output = %t, live = %t", signedOutput, targets.SignedReleaseOutput.Configured)
	}
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
