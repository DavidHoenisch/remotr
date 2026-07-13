package models

import "testing"

func TestMountResourceValidate(t *testing.T) {
	mounted := true
	force := UnmountForce
	enforce := true
	for _, test := range []struct {
		name  string
		value MountResource
		valid bool
	}{
		{name: "runtime and persistence", value: MountResource{Name: "cache", Source: "tmpfs", Target: "/var/cache/remotr", FilesystemType: "tmpfs", Options: []string{"mode=0755"}, Mounted: &mounted}, valid: true},
		{name: "forced without authorization", value: MountResource{Name: "cache", Source: "tmpfs", Target: "/var/cache/remotr", FilesystemType: "tmpfs", Mounted: boolPointer(false), UnmountMode: force}},
		{name: "forced authorized", value: MountResource{ResourceMeta: ResourceMeta{Enforce: &enforce}, Name: "cache", Source: "tmpfs", Target: "/var/cache/remotr", FilesystemType: "tmpfs", Mounted: boolPointer(false), UnmountMode: force}, valid: true},
		{name: "unsafe option", value: MountResource{Name: "cache", Source: "tmpfs", Target: "/var/cache/remotr", FilesystemType: "tmpfs", Options: []string{"rw,exec"}, Mounted: &mounted}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.value.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Validate() error = %v, valid = %t", err, test.valid)
			}
		})
	}
}

func TestMountResourceNormalizedOptions(t *testing.T) {
	resource := MountResource{Options: []string{"rw", "mode=0755", "rw"}}
	got := resource.NormalizedOptions()
	if len(got) != 2 || got[0] != "mode=0755" || got[1] != "rw" {
		t.Fatalf("NormalizedOptions() = %v", got)
	}
}

func boolPointer(value bool) *bool { return &value }
