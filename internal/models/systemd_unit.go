package models

import (
	"fmt"
	"regexp"
	"strings"
)

// SystemdUnitResource owns one named systemd unit file or one named drop-in.
type SystemdUnitResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Unit         string `yaml:"unit"`
	DropIn       string `yaml:"dropIn,omitempty"`
	Content      string `yaml:"content,omitempty"`
	Mode         []int  `yaml:"mode,omitempty"`
	Owner        string `yaml:"owner,omitempty"`
	Group        string `yaml:"group,omitempty"`
}

var systemdDropInIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*[.]conf$`)

func (r SystemdUnitResource) Validate() error {
	if !endpointScheduleName.MatchString(r.Name) {
		return fmt.Errorf("systemd unit resource name %q is invalid", r.Name)
	}
	if !serviceIdentity.MatchString(r.Unit) || !strings.Contains(r.Unit, ".") {
		return fmt.Errorf("systemd unit identity %q is invalid", r.Unit)
	}
	if r.DropIn != "" && !systemdDropInIdentity.MatchString(r.DropIn) {
		return fmt.Errorf("systemd unit dropIn %q is invalid", r.DropIn)
	}
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("systemd unit lifecycle %q is unsupported", lifecycle)
	}
	if lifecycle == LifecycleAbsent {
		if r.Content != "" || len(r.Mode) != 0 || r.Owner != "" || r.Group != "" {
			return fmt.Errorf("absent systemd unit may declare only name, unit, dropIn, lifecycle, and shared metadata")
		}
		return nil
	}
	if r.Content == "" {
		return fmt.Errorf("systemd unit content is required")
	}
	if len(r.Content) > 1<<20 || strings.ContainsRune(r.Content, '\x00') {
		return fmt.Errorf("systemd unit content must be at most 1 MiB and contain no NUL")
	}
	if len(r.Mode) > 1 || len(r.Mode) == 1 && (r.Mode[0] < 0 || r.Mode[0] > 0o777) {
		return fmt.Errorf("systemd unit mode must contain one permission value")
	}
	for label, identity := range map[string]string{"owner": r.Owner, "group": r.Group} {
		if identity != "" && !scheduleUserName.MatchString(identity) {
			return fmt.Errorf("systemd unit %s %q is invalid", label, identity)
		}
	}
	return nil
}
