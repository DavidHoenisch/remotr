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
	expected := desktopScenarioIDs(t, root)
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
			ParityClaim      string   `json:"parityClaim"`
			PlannedParity    int      `json:"plannedParity"`
			OperatingSystems []string `json:"operatingSystems"`
			Architectures    []string `json:"architectures"`
			PackageFormats   []string `json:"packageFormats"`
			Publication      string   `json:"publication"`
			ReleaseEligible  bool     `json:"releaseEligible"`
			SignedOutput     bool     `json:"signedOutput"`
		} `json:"claims"`
		Scenarios []scenarioReview `json:"scenarios"`
	}
	reviewPath := filepath.Join(root, "openspec", "changes", "add-desktop-admin-app", "evidence", "14.5-scenario-review.json")
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
		review.Claims.Publication, review.Claims.ReleaseEligible, review.Claims.SignedOutput)
}

func desktopScenarioIDs(t *testing.T, root string) map[string]bool {
	t.Helper()
	pattern := regexp.MustCompile(`verification-id:\s*(OS-(?:DOA|DFV|DFA)-[0-9]{3})`)
	ids := map[string]bool{}
	for _, capability := range []string{"desktop-operator-access", "desktop-fleet-visibility", "desktop-fleet-actions"} {
		path := filepath.Join(root, "openspec", "changes", "add-desktop-admin-app", "specs", capability, "spec.md")
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

func assertDesktopReviewClaimsMatchInventories(t *testing.T, root, parityClaim string, plannedParity int, operatingSystems, architectures, packageFormats []string, publication string, releaseEligible, signedOutput bool) {
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
	if len(targets.Artifacts) != 1 {
		t.Fatalf("live desktop package target count = %d, want one", len(targets.Artifacts))
	}
	target := targets.Artifacts[0]
	sort.Strings(operatingSystems)
	sort.Strings(architectures)
	sort.Strings(packageFormats)
	if strings.Join(operatingSystems, ",") != target.OS || strings.Join(architectures, ",") != target.Architecture || strings.Join(packageFormats, ",") != target.PackageFormat {
		t.Errorf("review target claims = %v/%v/%v, live target = %s/%s/%s", operatingSystems, architectures, packageFormats, target.OS, target.Architecture, target.PackageFormat)
	}
	if publication != target.Publication || releaseEligible != target.ReleaseEligible || signedOutput != targets.SignedReleaseOutput.Configured {
		t.Errorf("review publication claims = %q/release=%t/signed=%t, live = %q/release=%t/signed=%t", publication, releaseEligible, signedOutput, target.Publication, target.ReleaseEligible, targets.SignedReleaseOutput.Configured)
	}
}
