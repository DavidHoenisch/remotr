package models

import "testing"

func TestTimeSyncResourceValidate(t *testing.T) {
	enabled := true
	for _, test := range []struct {
		name  string
		value TimeSyncResource
		valid bool
	}{
		{name: "enablement", value: TimeSyncResource{Name: "ntp", Provider: TimeSyncProviderSystemdTimesyncd, Enabled: &enabled}, valid: true},
		{name: "no managed scope", value: TimeSyncResource{Name: "empty", Provider: TimeSyncProviderSystemdTimesyncd}},
		{name: "unknown provider", value: TimeSyncResource{Name: "chrony", Provider: "chrony", Enabled: &enabled}},
		{name: "unsafe server", value: TimeSyncResource{Name: "bad-server", Provider: TimeSyncProviderSystemdTimesyncd, Servers: []string{"time.example.test;rm"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.value.Validate()
			if test.valid && err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}
