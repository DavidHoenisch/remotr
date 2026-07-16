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
	SchemaVersion  int         `json:"schema_version"`
	Source         Source      `json:"source"`
	Publication    Publication `json:"publication"`
	StatusValues   []string    `json:"status_values"`
	SelectorPolicy string      `json:"selector_policy"`
	Entries        []Entry     `json:"entries"`
}

type Source struct {
	Binary       string `json:"binary"`
	CommandTree  string `json:"command_tree"`
	Visibility   string `json:"visibility"`
	CommandCount int    `json:"command_count"`
}

type Publication struct {
	Updated     string `json:"updated"`
	ParityClaim string `json:"parity_claim"`
	Gate        string `json:"gate"`
}

type Entry struct {
	Command              string   `json:"command"`
	DesktopWorkflow      string   `json:"desktop_workflow"`
	Status               string   `json:"status"`
	TargetFeatureRelease string   `json:"target_feature_release"`
	VerificationIDs      []string `json:"verification_ids"`
	PassingSelectors     []string `json:"passing_selectors"`
	ReviewedReason       string   `json:"reviewed_reason,omitempty"`
	InterfaceMechanic    bool     `json:"interface_mechanic,omitempty"`
	DesktopDifference    string   `json:"desktop_difference,omitempty"`
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
		if entry.Status == "planned" && len(entry.PassingSelectors) != 0 {
			issues = append(issues, "planned command claims passing selectors: "+entry.Command)
		}
		if entry.Status == "not_applicable" && strings.TrimSpace(entry.ReviewedReason) == "" {
			issues = append(issues, "not-applicable command has no reviewed reason: "+entry.Command)
		}
		if entry.Status == "not_applicable" && len(entry.PassingSelectors) != 0 {
			issues = append(issues, "not-applicable command claims passing selectors: "+entry.Command)
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

// ValidatePublished applies release-publication checks in addition to the
// entry-level parity drift checks performed by Validate.
func ValidatePublished(commandPaths []string, inventory Inventory) []string {
	issues := Validate(commandPaths, inventory)
	if inventory.SchemaVersion != 1 {
		issues = append(issues, fmt.Sprintf("desktop parity schema version = %d, want 1", inventory.SchemaVersion))
	}
	if inventory.Source.CommandCount != len(commandPaths) {
		issues = append(issues, fmt.Sprintf("published command count = %d, want %d", inventory.Source.CommandCount, len(commandPaths)))
	}
	if strings.TrimSpace(inventory.Publication.Updated) == "" {
		issues = append(issues, "desktop parity publication has no update date")
	}
	if strings.TrimSpace(inventory.Publication.Gate) == "" {
		issues = append(issues, "desktop parity publication has no gate command")
	}
	if inventory.Publication.ParityClaim != "partial" && inventory.Publication.ParityClaim != "full" {
		issues = append(issues, "desktop parity publication has invalid parity claim: "+inventory.Publication.ParityClaim)
	}

	current := make(map[string]struct{}, len(commandPaths))
	for _, command := range commandPaths {
		current[command] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Entries))
	planned := false
	for _, entry := range inventory.Entries {
		if _, exists := seen[entry.Command]; exists {
			issues = append(issues, "duplicate desktop parity mapping: "+entry.Command)
		} else {
			seen[entry.Command] = struct{}{}
		}
		if _, exists := current[entry.Command]; !exists {
			issues = append(issues, "stale desktop parity mapping: "+entry.Command)
		}
		if entry.Status == "planned" {
			planned = true
		}
		if entry.Status == "not_applicable" && !entry.InterfaceMechanic {
			issues = append(issues, "not-applicable command is not classified as an interface mechanic: "+entry.Command)
		}
	}
	if inventory.Publication.ParityClaim == "full" && planned {
		issues = append(issues, "full desktop parity claim is invalid while planned workflows remain")
	}
	sort.Strings(issues)
	return issues
}
