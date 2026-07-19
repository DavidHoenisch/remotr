package traceability

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scenario is one canonical OpenSpec scenario, including its source location
// and any adjacent immutable verification-ID comment.
type Scenario struct {
	Change           string `json:"change"`
	Capability       string `json:"capability"`
	Operation        string `json:"operation,omitempty"`
	Requirement      string `json:"requirement"`
	Title            string `json:"scenario"`
	VerificationID   string `json:"verification_id,omitempty"`
	Path             string `json:"path"`
	ScenarioLine     int    `json:"scenario_line"`
	VerificationLine int    `json:"verification_line,omitempty"`
}

// Inventory discovers scenarios from active and archived change specifications
// below an OpenSpec root. It never relies on a separate scenario list.
func Inventory(root string) ([]Scenario, error) {
	changesRoot := filepath.Join(root, "changes")
	var specs []string
	if err := filepath.WalkDir(changesRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "spec.md" {
			specs = append(specs, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(specs)

	var scenarios []Scenario
	for _, spec := range specs {
		change, capability, err := specSource(changesRoot, spec)
		if err != nil {
			return nil, err
		}
		found, err := inventorySpec(spec, change, capability)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, found...)
	}
	return scenarios, nil
}

func specSource(changesRoot, spec string) (string, string, error) {
	relative, err := filepath.Rel(changesRoot, spec)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("invalid OpenSpec path %q", spec)
	}
	if parts[0] == "archive" {
		parts = parts[1:]
	}
	if len(parts) != 4 || parts[1] != "specs" || parts[3] != "spec.md" {
		return "", "", fmt.Errorf("invalid OpenSpec scenario source %q", spec)
	}
	return parts[0], parts[2], nil
}

func inventorySpec(path, change, capability string) ([]Scenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var scenarios []Scenario
	var operation string
	var requirement string
	pendingID, pendingIDLine := "", 0
	scanner := bufio.NewScanner(f)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()
		switch {
		case line == "## ADDED Requirements":
			operation = "added"
		case line == "## MODIFIED Requirements":
			operation = "modified"
		case line == "## REMOVED Requirements":
			operation = "removed"
		case line == "## RENAMED Requirements":
			operation = "renamed"
		case strings.HasPrefix(line, "### Requirement: "):
			requirement = strings.TrimPrefix(line, "### Requirement: ")
		case strings.HasPrefix(line, "#### Scenario: "):
			scenarios = append(scenarios, Scenario{
				Change:           change,
				Capability:       capability,
				Operation:        operation,
				Requirement:      requirement,
				Title:            strings.TrimPrefix(line, "#### Scenario: "),
				VerificationID:   pendingID,
				Path:             path,
				ScenarioLine:     lineNumber,
				VerificationLine: pendingIDLine,
			})
			pendingID, pendingIDLine = "", 0
		case isVerificationComment(line):
			id := strings.TrimSuffix(strings.TrimPrefix(line, "<!-- verification-id: "), " -->")
			if len(scenarios) > 0 && scenarios[len(scenarios)-1].VerificationID == "" {
				scenarios[len(scenarios)-1].VerificationID = id
				scenarios[len(scenarios)-1].VerificationLine = lineNumber
			} else {
				pendingID, pendingIDLine = id, lineNumber
			}
		}
	}
	return scenarios, scanner.Err()
}

func isVerificationComment(line string) bool {
	return strings.HasPrefix(line, "<!-- verification-id: ") && strings.HasSuffix(line, " -->")
}
