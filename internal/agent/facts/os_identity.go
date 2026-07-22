package facts

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// ErrAmbiguousOSRelease classifies malformed or duplicate exact identity data
// without requiring callers to inspect raw os-release content.
var ErrAmbiguousOSRelease = errors.New("ambiguous operating-system release data")

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
		return Facts{}, fmt.Errorf("%w: parse /etc/os-release: %v", ErrAmbiguousOSRelease, err)
	}

	consistent := true
	sourceCount := 1
	if usrData, usrErr := source.ReadFile("/usr/lib/os-release"); usrErr == nil {
		sourceCount = 2
		usr, parseErr := parseOSRelease(usrData)
		if parseErr != nil {
			return Facts{}, fmt.Errorf("%w: parse /usr/lib/os-release: %v", ErrAmbiguousOSRelease, parseErr)
		}
		consistent = etc.ID == usr.ID && etc.VersionID == usr.VersionID
	}

	var distro types.Distro
	switch etc.ID {
	case "ubuntu":
		distro = types.Ubuntu
	case "debian":
		distro = types.Debian
	case "pop":
		distro = types.PopOS
	case "arch":
		distro = types.Arch
	default:
		if slices.Contains(etc.IDLike, "debian") || slices.Contains(etc.IDLike, "ubuntu") {
			distro = types.Debian
		} else {
			return Facts{}, fmt.Errorf("unsupported distro ID %q", etc.ID)
		}
	}

	vendor := ""
	if distro == types.Ubuntu {
		output, runErr := source.Run("/usr/bin/dpkg-vendor", "--query", "Vendor")
		if runErr != nil {
			origin, readErr := source.ReadFile("/etc/dpkg/origins/default")
			if readErr != nil {
				return Facts{}, fmt.Errorf("query dpkg vendor and read default origin: %w", runErr)
			}
			output, readErr = parseDpkgOriginVendor(origin)
			if readErr != nil {
				return Facts{}, fmt.Errorf("parse default dpkg origin: %w", readErr)
			}
		}
		vendor = strings.TrimSpace(string(output))
	}

	return (Facts{
		Distro:               distro,
		DistroVersion:        etc.VersionID,
		OSID:                 etc.ID,
		OSIDLike:             etc.IDLike,
		OSReleaseSourceCount: sourceCount,
		OSReleaseConsistent:  consistent,
		DistroVendor:         vendor,
	}).Normalized(), nil
}

func parseDpkgOriginVendor(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 4096 {
		return nil, errors.New("invalid origin size")
	}
	var vendor string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "Vendor" {
			continue
		}
		if vendor != "" {
			return nil, errors.New("duplicate Vendor field")
		}
		vendor = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("scan origin")
	}
	if vendor == "" || len(vendor) > 256 {
		return nil, errors.New("missing or invalid Vendor field")
	}
	return []byte(vendor), nil
}

type localIdentitySource struct{}

func (localIdentitySource) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (localIdentitySource) Run(path string, args ...string) ([]byte, error) {
	return exec.Command(path, args...).Output()
}

// ExactUbuntu reports the complete local identity predicate required before an
// Ubuntu-only provider may consider release- and capability-specific evidence.
func (f Facts) ExactUbuntu() bool {
	return f.ExactUbuntuReason() == ""
}

// ExactUbuntuReason returns a bounded local reason when exact Canonical Ubuntu
// identity is not established. It deliberately excludes raw source contents.
func (f Facts) ExactUbuntuReason() string {
	if f.OSID != "ubuntu" {
		return fmt.Sprintf("exact distribution ID %q is not ubuntu", f.OSID)
	}
	if !f.OSReleaseConsistent {
		return "operating-system release sources disagree"
	}
	if f.DistroVersion == "" {
		return "exact Ubuntu release is missing"
	}
	if f.DistroVendor != "Ubuntu" {
		return "dpkg vendor is not Ubuntu"
	}
	if f.Distro != types.Ubuntu {
		return "portable distribution conflicts with exact Ubuntu identity"
	}
	return ""
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
		if len(value) > 256 {
			return osRelease{}, fmt.Errorf("%s exceeds 256 bytes", key)
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
