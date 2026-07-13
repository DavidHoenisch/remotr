package models

import (
	"fmt"
	"regexp"
	"strings"
)

var hostnameResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var hostnameValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$|^[A-Za-z0-9]$`)

func (h HostnameResource) Validate() error {
	if !hostnameResourceName.MatchString(h.Name) {
		return fmt.Errorf("hostname resource name %q is invalid", h.Name)
	}
	if h.Static == nil && h.Transient == nil {
		return fmt.Errorf("hostname resource requires static or transient state")
	}
	for _, value := range []*string{h.Static, h.Transient} {
		if value != nil && (!hostnameValue.MatchString(*value) || strings.HasPrefix(*value, ".") || strings.HasSuffix(*value, ".")) {
			return fmt.Errorf("hostname value %q is invalid", *value)
		}
	}
	return nil
}
