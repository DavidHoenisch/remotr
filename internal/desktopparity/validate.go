package desktopparity

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const MaxInventoryBytes = 1 << 20

type Inventory struct {
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Command              string   `json:"command"`
	Status               string   `json:"status"`
	TargetFeatureRelease string   `json:"target_feature_release"`
	VerificationIDs      []string `json:"verification_ids"`
	PassingSelectors     []string `json:"passing_selectors"`
	ReviewedReason       string   `json:"reviewed_reason,omitempty"`
}

func Load(path string) (Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Inventory{}, fmt.Errorf("read inventory: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (Inventory, error) {
	if len(data) > MaxInventoryBytes {
		return Inventory{}, fmt.Errorf("inventory exceeds %d bytes", MaxInventoryBytes)
	}
	var inventory Inventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("parse inventory: %w", err)
	}
	return inventory, nil
}

func Validate(commandPaths []string, inventory Inventory) []string {
	var issues []string
	mapped := make(map[string]struct{}, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		mapped[entry.Command] = struct{}{}
		switch entry.Status {
		case "implemented", "planned", "not_applicable":
		default:
			issues = append(issues, "command has invalid desktop disposition "+entry.Status+": "+entry.Command)
		}
		if len(entry.VerificationIDs) == 0 {
			issues = append(issues, "command has no OpenSpec verification IDs: "+entry.Command)
		}
		if entry.Status == "implemented" && len(entry.PassingSelectors) == 0 {
			issues = append(issues, "implemented command has no passing selectors: "+entry.Command)
		}
		if entry.Status == "planned" && strings.TrimSpace(entry.TargetFeatureRelease) == "" {
			issues = append(issues, "planned command has no target feature release: "+entry.Command)
		}
		if entry.Status == "not_applicable" && strings.TrimSpace(entry.ReviewedReason) == "" {
			issues = append(issues, "not-applicable command has no reviewed reason: "+entry.Command)
		}
	}

	for _, command := range commandPaths {
		if _, ok := mapped[command]; !ok {
			issues = append(issues, "unmapped non-hidden CLI command: "+command)
		}
	}
	sort.Strings(issues)
	return issues
}
