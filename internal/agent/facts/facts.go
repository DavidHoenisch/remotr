package facts

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// Facts are local OS properties used for in-document targeting.
type Facts struct {
	Distro               types.Distro
	DistroFamily         DistroFamily
	DistroVersion        string
	OSID                 string
	OSIDLike             []string
	OSReleaseSourceCount int
	OSReleaseConsistent  bool
	DistroVendor         string
	Arch                 types.Architecture
	Init                 InitBackend
	Package              types.PackageManager
	Firewall             FirewallBackend
	Network              NetworkBackend
	Security             SecurityBackend
	Desktop              []DesktopBackend
	Browser              []BrowserBackend
}

// Read collects distro and architecture from the local system.
func Read() (Facts, error) {
	identity, err := ReadIdentity(localIdentitySource{})
	if err != nil {
		return Facts{}, err
	}
	a, err := ReadArch()
	if err != nil {
		return Facts{}, err
	}
	identity.Arch = a
	return detectLocalBackends(identity), nil
}

// ReadDistro maps /etc/os-release ID to a supported Distro.
func ReadDistro() (types.Distro, error) {
	distro, _, err := readDistroVersion()
	return distro, err
}

func readDistroVersion() (types.Distro, string, error) {
	identity, err := ReadIdentity(localIdentitySource{})
	if err != nil {
		return "", "", err
	}
	return identity.Distro, identity.DistroVersion, nil
}

// ReadArch maps uname -m to Architecture.
func ReadArch() (types.Architecture, error) {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", fmt.Errorf("uname -m: %w", err)
	}
	return mapMachine(strings.TrimSpace(string(out))), nil
}

func mapMachine(m string) types.Architecture {
	switch m {
	case "x86_64", "i686", "i386", "amd64":
		return types.X86
	case "aarch64", "arm64", "armv7l", "armv8l":
		return types.Arm
	default:
		if strings.Contains(m, "arm") {
			return types.Arm
		}
		return types.X86
	}
}

// PackageManagerForDistro returns the default package manager for a distro.
func PackageManagerForDistro(d types.Distro) types.PackageManager {
	switch d {
	case types.Arch:
		return types.Pacman
	default:
		return types.Apt
	}
}
