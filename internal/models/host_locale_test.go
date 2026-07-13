package models

import "testing"

func TestHostLocaleResourceValidate(t *testing.T) {
	timezone := "UTC"
	for _, test := range []struct {
		name  string
		value HostLocaleResource
		valid bool
	}{
		{name: "timezone only", value: HostLocaleResource{Name: "utc", Timezone: &timezone}, valid: true},
		{name: "no managed scope", value: HostLocaleResource{Name: "empty"}},
		{name: "empty locale map", value: HostLocaleResource{Name: "empty-locale", Locale: map[string]string{}}},
		{name: "invalid locale variable", value: HostLocaleResource{Name: "invalid-var", Locale: map[string]string{"PATH": "/bin"}}},
		{name: "invalid timezone", value: HostLocaleResource{Name: "bad-zone", Timezone: stringPtr("not/a timezone")}},
		{name: "unsupported lifecycle", value: HostLocaleResource{ResourceMeta: ResourceMeta{Lifecycle: LifecycleAbsent}, Name: "absent", Timezone: &timezone}},
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

func stringPtr(value string) *string { return &value }
