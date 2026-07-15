package facts

import (
	"os"
	"os/exec"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/types"
)

type DistroFamily string

const (
	DistroFamilyDebian DistroFamily = "debian"
	DistroFamilyArch   DistroFamily = "arch"
)

type InitBackend string

const (
	InitSystemd InitBackend = "systemd"
	InitOpenRC  InitBackend = "openrc"
	InitSysV    InitBackend = "sysv"
)

type FirewallBackend string

const (
	FirewallFirewalld FirewallBackend = "firewalld"
	FirewallNftables  FirewallBackend = "nftables"
)

type NetworkBackend string

const (
	NetworkManager        NetworkBackend = "network-manager"
	NetworkSystemdNetwork NetworkBackend = "systemd-networkd"
	NetworkNetplan        NetworkBackend = "netplan"
)

type SecurityBackend string

const (
	SecurityAppArmor SecurityBackend = "apparmor"
	SecuritySELinux  SecurityBackend = "selinux"
)

type DesktopBackend string

const (
	DesktopDconf     DesktopBackend = "dconf"
	DesktopGSettings DesktopBackend = "gsettings"
)

type BrowserBackend string

const (
	BrowserChromium     BrowserBackend = "chromium"
	BrowserGoogleChrome BrowserBackend = "google-chrome"
	BrowserFirefox      BrowserBackend = "firefox"
)

// Normalized fills portable facts derived from distro identity and returns
// deterministic, duplicate-free multi-valued backend facts.
func (f Facts) Normalized() Facts {
	switch f.Distro {
	case types.Debian, types.Ubuntu:
		f.DistroFamily = DistroFamilyDebian
	case types.Arch:
		f.DistroFamily = DistroFamilyArch
	}
	if f.Package == "" {
		f.Package = PackageManagerForDistro(f.Distro)
	}
	sort.Slice(f.Desktop, func(i, j int) bool { return f.Desktop[i] < f.Desktop[j] })
	unique := f.Desktop[:0]
	for _, backend := range f.Desktop {
		if len(unique) == 0 || unique[len(unique)-1] != backend {
			unique = append(unique, backend)
		}
	}
	f.Desktop = unique
	sort.Slice(f.Browser, func(i, j int) bool { return f.Browser[i] < f.Browser[j] })
	uniqueBrowsers := f.Browser[:0]
	for _, browser := range f.Browser {
		if len(uniqueBrowsers) == 0 || uniqueBrowsers[len(uniqueBrowsers)-1] != browser {
			uniqueBrowsers = append(uniqueBrowsers, browser)
		}
	}
	f.Browser = uniqueBrowsers
	return f
}

func detectLocalBackends(f Facts) Facts {
	if executableExists("systemctl") {
		f.Init = InitSystemd
	} else if executableExists("rc-service") {
		f.Init = InitOpenRC
	} else if executableExists("service") {
		f.Init = InitSysV
	}
	if executableExists("firewall-cmd") {
		f.Firewall = FirewallFirewalld
	} else if executableExists("nft") {
		f.Firewall = FirewallNftables
	}
	f.Network = detectNetworkBackend(executableExists)
	if executableExists("aa-status") || pathExists("/sys/module/apparmor") {
		f.Security = SecurityAppArmor
	} else if executableExists("getenforce") || pathExists("/sys/fs/selinux") {
		f.Security = SecuritySELinux
	}
	if executableExists("dconf") {
		f.Desktop = append(f.Desktop, DesktopDconf)
	}
	if executableExists("gsettings") {
		f.Desktop = append(f.Desktop, DesktopGSettings)
	}
	if executableExists("chromium") || executableExists("chromium-browser") {
		f.Browser = append(f.Browser, BrowserChromium)
	}
	if executableExists("google-chrome") || executableExists("google-chrome-stable") {
		f.Browser = append(f.Browser, BrowserGoogleChrome)
	}
	if executableExists("firefox") {
		f.Browser = append(f.Browser, BrowserFirefox)
	}
	return f.Normalized()
}

func detectNetworkBackend(available func(string) bool) NetworkBackend {
	if available("nmcli") {
		return NetworkManager
	}
	if available("netplan") {
		return NetworkNetplan
	}
	if available("networkctl") {
		return NetworkSystemdNetwork
	}
	return ""
}

func executableExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
