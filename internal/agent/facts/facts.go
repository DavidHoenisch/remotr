package facts

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// Facts are local OS properties used for in-document targeting.
type Facts struct {
	Distro types.Distro
	Arch   types.Architecture
}

// Read collects distro and architecture from the local system.
func Read() (Facts, error) {
	d, err := ReadDistro()
	if err != nil {
		return Facts{}, err
	}
	a, err := ReadArch()
	if err != nil {
		return Facts{}, err
	}
	return Facts{Distro: d, Arch: a}, nil
}

// ReadDistro maps /etc/os-release ID to a supported Distro.
func ReadDistro() (types.Distro, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("open os-release: %w", err)
	}
	defer f.Close()

	id := ""
	idLike := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`)
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}

	switch id {
	case "debian":
		return types.Debian, nil
	case "ubuntu":
		return types.Ubuntu, nil
	case "arch":
		return types.Arch, nil
	}
	if strings.Contains(idLike, "debian") {
		return types.Debian, nil
	}
	return "", fmt.Errorf("unsupported distro ID %q", id)
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
