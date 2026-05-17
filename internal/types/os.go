package types

// Defines the linux distrobution
type Distro string

const (
	Ubuntu Distro = "Ubuntu"
	Arch   Distro = "Arch"
	Debian Distro = "Debian"
)

// Defines the CPU architecture.
// This is useful for cases where
// software installs are architecture
// dependant
type Architecture string

const (
	X86 Architecture = "x86"
	Arm Architecture = "ARM"
)

// Defines the package manager used by a
// Linux distrobution
type PackageManager string

const (
	Apt    PackageManager = "apt"
	Pacman PackageManager = "pacman"
	Yay    PackageManager = "yay"
)
