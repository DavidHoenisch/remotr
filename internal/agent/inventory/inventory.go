package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os/exec"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/go-sysinfo/firewalld"
	"github.com/DavidHoenisch/go-sysinfo/nftables"
)

// Snapshot is a JSON-serializable machine inventory report.
type Snapshot struct {
	OSRelease    OSReleaseInfo     `json:"osRelease"`
	CPU          CPUInfo           `json:"cpu"`
	RAM          RAMInfo           `json:"ram"`
	Screen       ScreenInfo        `json:"screen"`
	Networks     []NetworkInfo     `json:"networks,omitempty"`
	Batteries    []BatteryInfo     `json:"batteries,omitempty"`
	BlockDevices []BlockDeviceInfo `json:"blockDevices,omitempty"`
	Kernel       KernelInfo        `json:"kernel"`
	TPM          TPMInfo           `json:"tpm"`
	Firewall     FirewallInfo      `json:"firewall,omitempty"`
}

type OSReleaseInfo struct {
	Name       string `json:"name,omitempty"`
	PrettyName string `json:"prettyName,omitempty"`
	ID         string `json:"id,omitempty"`
	VersionID  string `json:"versionId,omitempty"`
}

type CPUInfo struct {
	ModelName string `json:"modelName,omitempty"`
	VendorID  string `json:"vendorId,omitempty"`
	CPUFamily string `json:"cpuFamily,omitempty"`
	Model     string `json:"model,omitempty"`
	CoreCount string `json:"coreCount,omitempty"`
}

type RAMInfo struct {
	MemTotal     string `json:"memTotal,omitempty"`
	MemFree      string `json:"memFree,omitempty"`
	MemAvailable string `json:"memAvailable,omitempty"`
}

type ScreenInfo struct {
	VirtualSize ScreenSize `json:"virtualSize"`
	Mode        string     `json:"mode,omitempty"`
}

type ScreenSize struct {
	X string `json:"x,omitempty"`
	Y string `json:"y,omitempty"`
}

type NetworkInfo struct {
	Name       string              `json:"name"`
	MACAddress string              `json:"macAddress,omitempty"`
	IPv4       []string            `json:"ipv4,omitempty"`
	IPv6       []string            `json:"ipv6,omitempty"`
	Statistics *NetworkStatistics  `json:"statistics,omitempty"`
}

type NetworkStatistics struct {
	Operstate string `json:"operstate,omitempty"`
	Mtu       string `json:"mtu,omitempty"`
	Speed     string `json:"speed,omitempty"`
	Carrier   string `json:"carrier,omitempty"`
}

type BlockDeviceInfo struct {
	Name           string `json:"name"`
	Size           string `json:"size,omitempty"`
	Model          string `json:"model,omitempty"`
	Encrypted      bool   `json:"encrypted"`
	EncryptionType string `json:"encryptionType,omitempty"`
}

type BatteryInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status,omitempty"`
	Capacity      string `json:"capacity,omitempty"`
	CapacityLevel string `json:"capacityLevel,omitempty"`
	PowerNow      string `json:"powerNow,omitempty"`
	Technology    string `json:"technology,omitempty"`
}

type KernelInfo struct {
	Version string `json:"version,omitempty"`
}

type TPMInfo struct {
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

// Collect gathers a machine inventory snapshot using the given sys reader.
func Collect(r gosysinfo.SysReader) Snapshot {
	snap := Snapshot{
		TPM: TPMInfo{
			Version:     gosysinfo.GetTpmVersion(r),
			Description: gosysinfo.GetTpmDescription(r),
		},
	}

	if k := gosysinfo.GetKernelVersion(r); k != nil {
		snap.Kernel = KernelInfo{Version: k.Version}
	}

	if os := gosysinfo.GetOSRelease(r); os != nil {
		snap.OSRelease = OSReleaseInfo{
			Name:       os.Name,
			PrettyName: os.PrettyName,
			ID:         os.ID,
			VersionID:  os.VersionID,
		}
	}

	if cpu := gosysinfo.GetCPU(r); cpu != nil {
		snap.CPU = CPUInfo{
			ModelName: cpu.ModelName,
			VendorID:  cpu.VendorID,
			CPUFamily: cpu.CPUFamily,
			Model:     cpu.Model,
			CoreCount: cpu.CoreCount,
		}
	}

	if ram := gosysinfo.GetRAM(r); ram != nil {
		snap.RAM = RAMInfo{
			MemTotal:     ram.MemTotal,
			MemFree:      ram.MemFree,
			MemAvailable: ram.MemAvailable,
		}
	}

	size := gosysinfo.GetScreenVirtualSize(r)
	mode := gosysinfo.GetScreenMode(r)
	snap.Screen = ScreenInfo{
		VirtualSize: ScreenSize{X: size.X, Y: size.Y},
		Mode:        mode.Value,
	}

	if ifaces, err := gosysinfo.ListNetworkInterfaces(); err == nil {
		for _, name := range ifaces {
			net := NetworkInfo{Name: name}
			if conn, err := gosysinfo.GetNetworkConnectionInfo(r, name); err == nil && conn != nil {
				net.MACAddress = conn.MACAddress
			}
			if ips, err := gosysinfo.GetNetworkIPInfo(r, name); err == nil && ips != nil {
				net.IPv4 = ips.IPv4
				net.IPv6 = ips.IPv6
			}
			if stats, err := gosysinfo.GetNetworkStatistics(r, name); err == nil && stats != nil {
				net.Statistics = &NetworkStatistics{
					Operstate: stats.Operstate,
					Mtu:       stats.Mtu,
					Speed:     stats.Speed,
					Carrier:   stats.Carrier,
				}
			}
			snap.Networks = append(snap.Networks, net)
		}
	}

	if devices, err := gosysinfo.ListBlockDevices(); err == nil {
		for _, name := range devices {
			dev := BlockDeviceInfo{Name: name}
			if info, err := gosysinfo.GetBlockDeviceInfo(r, name); err == nil && info != nil {
				dev.Size = info.Size
				dev.Model = info.Model
				dev.Encrypted = info.Encrypted
				dev.EncryptionType = info.EncryptionType
			}
			snap.BlockDevices = append(snap.BlockDevices, dev)
		}
	}

	if batteries, err := gosysinfo.ListBatteries(); err == nil {
		for _, name := range batteries {
			bat := BatteryInfo{Name: name}
			if info, err := gosysinfo.GetBatteryInfo(r, name); err == nil && info != nil {
				bat.Status = info.Status
				bat.Capacity = info.Capacity
				bat.CapacityLevel = info.CapacityLevel
				bat.PowerNow = info.PowerNow
				bat.Technology = info.Technology
			}
			snap.Batteries = append(snap.Batteries, bat)
		}
	}

	snap.Firewall = collectFirewall()
	return snap
}

type FirewallInfo struct {
	Backend   string          `json:"backend,omitempty"`
	Firewalld *FirewalldInfo  `json:"firewalld,omitempty"`
	Nftables  *NftablesInfo   `json:"nftables,omitempty"`
}

type FirewalldInfo struct {
	DefaultZone string              `json:"defaultZone,omitempty"`
	Zones       []FirewalldZoneInfo `json:"zones,omitempty"`
}

type FirewalldZoneInfo struct {
	Name      string   `json:"name"`
	Target    string   `json:"target,omitempty"`
	Services  []string `json:"services,omitempty"`
	Ports     []string `json:"ports,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	RichRules []string `json:"richRules,omitempty"`
}

type NftablesInfo struct {
	RawRuleset string `json:"rawRuleset,omitempty"`
}

func collectFirewall() FirewallInfo {
	info := FirewallInfo{}

	fwReader := firewalld.Reader{}
	if fwReader.Available() {
		info.Backend = "firewalld"
		summary, err := fwReader.GetRulesetSummary()
		if err == nil && summary != nil {
			fwInfo := &FirewalldInfo{
				DefaultZone: summary.DefaultZone,
			}
			for _, z := range summary.Zones {
				fwInfo.Zones = append(fwInfo.Zones, FirewalldZoneInfo{
					Name:      z.Name,
					Target:    z.Target,
					Services:  z.Services,
					Ports:     z.Ports,
					Sources:   z.Sources,
					RichRules: z.RichRules,
				})
			}
			info.Firewalld = fwInfo
		}
		return info
	}

	nftReader := nftables.Reader{}
	if nftReader.Available() {
		info.Backend = "nftables"
		// Use exec.Command directly for raw ruleset; go-sysinfo only exposes summaries.
		if out, err := exec.Command("nft", "list", "ruleset").Output(); err == nil {
			info.Nftables = &NftablesInfo{RawRuleset: string(out)}
		}
		return info
	}

	return info
}

// MarshalJSON returns canonical JSON bytes for a snapshot.
func MarshalJSON(s Snapshot) ([]byte, error) {
	return json.Marshal(s)
}

// Digest returns a SHA-256 hex digest of the snapshot JSON for change detection.
func Digest(s Snapshot) (string, error) {
	raw, err := MarshalJSON(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
