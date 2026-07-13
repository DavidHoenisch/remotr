package models

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var timeSyncName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var timeSyncHost = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

const TimeSyncProviderSystemdTimesyncd = "systemd-timesyncd"

// Validate admits only providers with a complete, advertised contract. The
// initial systemd-timesyncd provider supports enablement and named NTP/pool
// fragments; other providers are not silently treated as partial support.
func (r TimeSyncResource) Validate() error {
	if !timeSyncName.MatchString(r.Name) {
		return fmt.Errorf("time sync resource name %q is invalid", r.Name)
	}
	if r.Provider != TimeSyncProviderSystemdTimesyncd {
		return fmt.Errorf("time sync provider %q is not advertised", r.Provider)
	}
	if r.Enabled == nil && r.Servers == nil && r.Pools == nil {
		return fmt.Errorf("time sync resource requires enablement, servers, or pools")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent {
		return fmt.Errorf("time sync lifecycle %q is unsupported", r.Lifecycle)
	}
	if err := validateTimeServers("servers", r.Servers); err != nil {
		return err
	}
	return validateTimeServers("pools", r.Pools)
}

func validateTimeServers(field string, servers []string) error {
	if len(servers) > 16 {
		return fmt.Errorf("time sync %s exceeds 16 entries", field)
	}
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != server || server == "" || (!timeSyncHost.MatchString(server) && net.ParseIP(server) == nil) {
			return fmt.Errorf("time sync %s entry %q is invalid", field, server)
		}
		if _, exists := seen[server]; exists {
			return fmt.Errorf("time sync %s entry %q is duplicated", field, server)
		}
		seen[server] = struct{}{}
	}
	return nil
}
