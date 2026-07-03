package firewalld

import (
	"strings"
)

type ZoneInfo struct {
	Name       string
	Default    bool
	Target     string
	Interfaces []string
	Services   []string
	Ports      []string
	Sources    []string
	RichRules  []string
}

type RulesetSummary struct {
	DefaultZone string
	Zones       []ZoneInfo
}

func Available(r Reader) bool {
	_, err := r.cmd().Run("firewall-cmd", "--version")
	return err == nil
}

func (r Reader) Available() bool {
	return Available(r)
}

func GetDefaultZone(r Reader) (string, error) {
	if !Available(r) {
		return "", nil
	}
	out, err := r.cmd().Run("firewall-cmd", "--get-default-zone")
	if err != nil {
		return handleError(r, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r Reader) GetDefaultZone() (string, error) {
	return GetDefaultZone(r)
}

func GetActiveZones(r Reader) ([]string, error) {
	if !Available(r) {
		return nil, nil
	}
	out, err := r.cmd().Run("firewall-cmd", "--get-active-zones")
	if err != nil {
		return handleSliceError(r, err)
	}
	return parseActiveZones(string(out)), nil
}

func (r Reader) GetActiveZones() ([]string, error) {
	return GetActiveZones(r)
}

func GetZoneInfo(r Reader, zone string) (*ZoneInfo, error) {
	if !Available(r) || zone == "" {
		return nil, nil
	}
	out, err := r.cmd().Run("firewall-cmd", "--zone="+zone, "--list-all")
	if err != nil {
		return handlePtrError[ZoneInfo](r, err)
	}
	info := parseZoneInfo(zone, string(out))
	return info, nil
}

func (r Reader) GetZoneInfo(zone string) (*ZoneInfo, error) {
	return GetZoneInfo(r, zone)
}

func GetRulesetSummary(r Reader) (*RulesetSummary, error) {
	if !Available(r) {
		return nil, nil
	}
	out, err := r.cmd().Run("firewall-cmd", "--list-all-zones")
	if err != nil {
		return handlePtrError[RulesetSummary](r, err)
	}
	defaultZone, err := GetDefaultZone(r)
	if err != nil {
		return nil, err
	}
	summary := parseRulesetSummary(string(out), defaultZone)
	return summary, nil
}

func (r Reader) GetRulesetSummary() (*RulesetSummary, error) {
	return GetRulesetSummary(r)
}

func handleError(r Reader, err error) (string, error) {
	if r.Privileged {
		return "", err
	}
	return "", nil
}

func handleSliceError(r Reader, err error) ([]string, error) {
	if r.Privileged {
		return nil, err
	}
	return nil, nil
}

func handlePtrError[T any](r Reader, err error) (*T, error) {
	if r.Privileged {
		return nil, err
	}
	return nil, nil
}

func parseActiveZones(content string) []string {
	var zones []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "interfaces:") {
			continue
		}
		zones = append(zones, line)
	}
	return zones
}

func parseRulesetSummary(content, defaultZone string) *RulesetSummary {
	summary := &RulesetSummary{DefaultZone: defaultZone}
	lines := strings.Split(content, "\n")
	var current *ZoneInfo

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			// New zone line
			if current != nil {
				summary.Zones = append(summary.Zones, *current)
			}
			current = &ZoneInfo{Name: strings.TrimSpace(line)}
			if strings.Contains(line, "(default)") {
				current.Default = true
				current.Name = strings.TrimSuffix(current.Name, " (default)")
			}
			continue
		}
		if current == nil {
			continue
		}
		key, values := parseKeyValue(trimmed)
		switch key {
		case "target":
			current.Target = values
		case "interfaces":
			current.Interfaces = splitValues(values)
		case "services":
			current.Services = splitValues(values)
		case "ports":
			current.Ports = splitValues(values)
		case "sources":
			current.Sources = splitValues(values)
		case "rich rules":
			if values != "" {
				current.RichRules = append(current.RichRules, values)
			}
		}
	}
	if current != nil {
		summary.Zones = append(summary.Zones, *current)
	}
	return summary
}

func parseZoneInfo(zone, content string) *ZoneInfo {
	info := &ZoneInfo{Name: zone}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, values := parseKeyValue(trimmed)
		switch key {
		case "target":
			info.Target = values
		case "interfaces":
			info.Interfaces = splitValues(values)
		case "services":
			info.Services = splitValues(values)
		case "ports":
			info.Ports = splitValues(values)
		case "sources":
			info.Sources = splitValues(values)
		case "rich rules":
			if values != "" {
				info.RichRules = append(info.RichRules, values)
			}
		}
	}
	return info
}

func parseKeyValue(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
}

func splitValues(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil
	}
	return parts
}
