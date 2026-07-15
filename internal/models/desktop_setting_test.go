package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestDesktopSettingValidationRejectsUnsupportedTypeAndScopeCombinations(t *testing.T) {
	base := models.DesktopSettingResource{
		Name: "animations", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Path:     "/org/gnome/desktop/interface/enable-animations",
		Value:    models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
	}
	tests := []struct {
		name   string
		mutate func(*models.DesktopSettingResource)
	}{
		{name: "boolean string", mutate: func(r *models.DesktopSettingResource) { r.Value.Value = "true" }},
		{name: "mandatory user", mutate: func(r *models.DesktopSettingResource) { r.Level = models.DesktopSettingLevelMandatory }},
		{name: "gsettings system", mutate: func(r *models.DesktopSettingResource) {
			r.Provider = models.DesktopSettingProviderGSettings
			r.Scope = models.DesktopSettingScopeSystem
		}},
		{name: "uint overflow", mutate: func(r *models.DesktopSettingResource) {
			r.Value = models.DesktopSettingValue{Type: models.DesktopValueUint32, Value: int64(1 << 33)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := base
			test.mutate(&resource)
			if err := resource.Validate(); err == nil {
				t.Fatal("Validate() accepted unsupported desktop setting")
			}
		})
	}
}

func TestParseStateCanonicalDesktopSetting(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: workstation
    resources:
      - kind: desktopSetting
        name: animations
        provider: gsettings
        scope: user
        selector: {mode: explicit, usernames: [alice]}
        schema: org.gnome.desktop.interface
        key: enable-animations
        value: {type: boolean, value: false}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].DesktopSettings) != 1 {
		t.Fatalf("state = %#v", state)
	}
	resource := state.Configurations[0].DesktopSettings[0]
	if resource.Value.Type != models.DesktopValueBoolean || resource.Value.Value != false {
		t.Fatalf("desktop value = %#v", resource.Value)
	}
}
