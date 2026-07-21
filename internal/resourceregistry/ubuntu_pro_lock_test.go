package resourceregistry_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

func TestUbuntuProCatalogedLockDomains(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{name: "ordinary", yaml: "services:\n  - {name: esm-apps, state: enabled}\n", want: []string{"ubuntu-pro", "package-manager:apt", "operator-custom"}},
		{name: "snap", yaml: "services:\n  - {name: anbox-cloud, state: enabled}\n", want: []string{"ubuntu-pro", "package-manager:apt", "package-manager:snap", "operator-custom"}},
		{name: "boot", yaml: "services:\n  - {name: realtime-kernel, state: enabled, variant: raspi}\n", want: []string{"ubuntu-pro", "package-manager:apt", "boot", "operator-custom"}},
	}
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document yaml.Node
			raw := "kind: ubuntuPro\nname: primary-subscription\nlifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nlockDomains: [operator-custom]\n" + test.yaml
			if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
				t.Fatal(err)
			}
			resource, err := registry.Decode(document.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := resource.Validate(); err != nil {
				t.Fatal(err)
			}
			locks := resource.LockDomains()
			for _, lock := range test.want {
				if !slices.Contains(locks, lock) {
					t.Errorf("LockDomains() = %v, missing %q", locks, lock)
				}
			}
		})
	}
}
