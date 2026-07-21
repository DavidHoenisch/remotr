package facts

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// IdentitySource is the operating-system boundary used to collect exact
// distribution identity without coupling tests to the host running them.
type IdentitySource interface {
	ReadFile(path string) ([]byte, error)
	Run(path string, args ...string) ([]byte, error)
}

// ReadIdentity preserves exact os-release and vendor facts separately from
// the portable distribution family used by generic providers.
func ReadIdentity(source IdentitySource) (Facts, error) {
	etcData, err := source.ReadFile("/etc/os-release")
	if err != nil {
		return Facts{}, fmt.Errorf("read /etc/os-release: %w", err)
	}
	etc, err := parseOSRelease(etcData)
	if err != nil {
		return Facts{}, fmt.Errorf("parse /etc/os-release: %w", err)
	}

	consistent := true
	if usrData, usrErr := source.ReadFile("/usr/lib/os-release"); usrErr == nil {
		usr, parseErr := parseOSRelease(usrData)
		if parseErr != nil {
			return Facts{}, fmt.Errorf("parse /usr/lib/os-release: %w", parseErr)
		}
		consistent = etc.ID == usr.ID && etc.VersionID == usr.VersionID
	}

	var distro types.Distro
	switch etc.ID {
	case "ubuntu":
		distro = types.Ubuntu
	case "debian":
		distro = types.Debian
	case "arch":
		distro = types.Arch
	default:
		return Facts{}, fmt.Errorf("unsupported distro ID %q", etc.ID)
	}

	vendor := ""
	if distro == types.Ubuntu || distro == types.Debian {
		output, runErr := source.Run("/usr/bin/dpkg-vendor", "--query", "Vendor")
		if runErr != nil {
			return Facts{}, fmt.Errorf("query dpkg vendor: %w", runErr)
		}
		vendor = strings.TrimSpace(string(output))
	}

	return (Facts{
		Distro:              distro,
		DistroVersion:       etc.VersionID,
		OSID:                etc.ID,
		OSIDLike:            etc.IDLike,
		OSReleaseConsistent: consistent,
		DistroVendor:        vendor,
	}).Normalized(), nil
}

type osRelease struct {
	ID        string
	VersionID string
	IDLike    []string
}

func parseOSRelease(data []byte) (osRelease, error) {
	result := osRelease{}
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return osRelease{}, fmt.Errorf("malformed assignment")
		}
		if key != "ID" && key != "VERSION_ID" && key != "ID_LIKE" {
			continue
		}
		if seen[key] {
			return osRelease{}, fmt.Errorf("duplicate %s", key)
		}
		seen[key] = true
		value, err := parseOSReleaseValue(raw)
		if err != nil {
			return osRelease{}, fmt.Errorf("%s: %w", key, err)
		}
		switch key {
		case "ID":
			result.ID = value
		case "VERSION_ID":
			result.VersionID = value
		case "ID_LIKE":
			result.IDLike = strings.Fields(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return osRelease{}, err
	}
	if result.ID == "" || result.VersionID == "" {
		return osRelease{}, fmt.Errorf("ID and VERSION_ID are required")
	}
	return result, nil
}

func parseOSReleaseValue(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty value")
	}
	if raw[0] != '"' {
		if strings.ContainsAny(raw, " \\'\"") {
			return "", fmt.Errorf("invalid unquoted value")
		}
		return raw, nil
	}
	if len(raw) < 2 || raw[len(raw)-1] != '"' {
		return "", fmt.Errorf("unterminated quoted value")
	}
	value := raw[1 : len(raw)-1]
	if strings.ContainsAny(value, "\n\r") {
		return "", fmt.Errorf("multiline value")
	}
	return value, nil
}
