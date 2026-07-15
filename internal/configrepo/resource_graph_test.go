package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestValidateState_rejectsInvalidDependencyGraphs(t *testing.T) {
	tests := []struct {
		name    string
		state   models.State
		wantErr string
	}{
		{
			name: "missing dependency",
			state: graphState(
				models.CommandResource{Name: "dependent", ResourceMeta: models.ResourceMeta{DependsOn: []string{"base/missing"}}},
			),
			wantErr: `resource "base/dependent" has unknown dependency "base/missing"`,
		},
		{
			name: "dependency is not a stable address",
			state: graphState(
				models.CommandResource{Name: "dependent", ResourceMeta: models.ResourceMeta{DependsOn: []string{"missing"}}},
			),
			wantErr: "must use stable <configuration>/<resource-name> address",
		},
		{
			name: "cycle",
			state: graphState(
				models.CommandResource{Name: "a", ResourceMeta: models.ResourceMeta{DependsOn: []string{"base/b"}}},
				models.CommandResource{Name: "b", ResourceMeta: models.ResourceMeta{DependsOn: []string{"base/a"}}},
			),
			wantErr: "dependency cycle detected: base/a -> base/b -> base/a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state, "test")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateState() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateState_acceptsCrossConfigurationStableDependency(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{
		{Name: "base", Commands: []models.CommandResource{{Name: "source"}}},
		{Name: "app", Commands: []models.CommandResource{{Name: "dependent", ResourceMeta: models.ResourceMeta{DependsOn: []string{"base/source"}}}}},
	}}
	if err := ValidateState(state, "test"); err != nil {
		t.Fatalf("ValidateState() error = %v", err)
	}
}

func TestValidateState_rejectsPolicyTrustReferenceToNonAnchor(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
		Name:     "base",
		Commands: []models.CommandResource{{Name: "not-an-anchor", Check: []string{"true"}}},
		BrowserPolicies: []models.BrowserPolicyResource{{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
			Name:         "homepage", Browser: models.BrowserChromium, PolicyName: "HomepageLocation",
			Scope: models.BrowserPolicyScopeSystem, Level: models.BrowserPolicyLevelMandatory,
			Value:        &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"},
			TrustAnchors: []string{"base/not-an-anchor"},
		}},
	}}}
	err := ValidateState(state, "test")
	if err == nil || !strings.Contains(err.Error(), `targets "command", not trustAnchor`) {
		t.Fatalf("ValidateState() error = %v", err)
	}
}

func graphState(resources ...models.CommandResource) models.State {
	return models.State{SchemaVersion: 1, Configurations: []models.Configuration{{Name: "base", Commands: resources}}}
}

func FuzzValidateStateDependencyGraph(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(0b111))
	f.Fuzz(func(t *testing.T, edges uint8) {
		dependency := func(bit uint8, address string) []string {
			if edges&(1<<bit) != 0 {
				return []string{address}
			}
			return nil
		}
		state := graphState(
			models.CommandResource{Name: "a", ResourceMeta: models.ResourceMeta{DependsOn: dependency(0, "base/b")}},
			models.CommandResource{Name: "b", ResourceMeta: models.ResourceMeta{DependsOn: dependency(1, "base/c")}},
			models.CommandResource{Name: "c", ResourceMeta: models.ResourceMeta{DependsOn: dependency(2, "base/a")}},
		)
		err := ValidateState(state, "fuzz")
		hasCycle := edges&0b111 == 0b111
		if hasCycle && (err == nil || !strings.Contains(err.Error(), "cycle")) {
			t.Fatalf("edges %03b form a cycle, error = %v", edges&0b111, err)
		}
		if !hasCycle && err != nil {
			t.Fatalf("edges %03b are acyclic, error = %v", edges&0b111, err)
		}
	})
}
