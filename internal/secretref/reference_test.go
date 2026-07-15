package secretref

import "testing"

func TestParseRequiresExplicitRemotrVersionSelector(t *testing.T) {
	tests := []struct {
		input    string
		name     string
		selector string
		valid    bool
	}{
		{input: "remotr:repositories/private@active", name: "repositories/private", selector: SelectorActive, valid: true},
		{input: "remotr:repositories/private@7", name: "repositories/private", selector: "7", valid: true},
		{input: "local-file:/run/secrets/private", name: "/run/secrets/private", valid: true},
		{input: "remotr:repositories/private"},
		{input: "remotr:repositories/private@latest"},
		{input: "remotr:repositories/private@0"},
		{input: "remotr:repositories/private@07"},
		{input: "remotr:repositories/private@-1"},
		{input: "remotr:repositories/private@active@7"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			reference, err := ParseSelected(tt.input)
			if !tt.valid {
				if err == nil {
					t.Fatalf("accepted %#v", reference)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if reference.Name != tt.name || reference.Selector != tt.selector {
				t.Fatalf("reference = %#v", reference)
			}
		})
	}
}

func FuzzParseSelectedReference(f *testing.F) {
	f.Add("remotr:repositories/private@active")
	f.Add("remotr:repositories/private@7")
	f.Add("remotr:repositories/private")
	f.Add("local-file:/run/secrets/private")
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1024 {
			return
		}
		reference, err := ParseSelected(input)
		if err != nil {
			return
		}
		if reference.Provider == ProviderRemotr && reference.Selector == "" {
			t.Fatal("accepted Remotr reference without a selector")
		}
	})
}
