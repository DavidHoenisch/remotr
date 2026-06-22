package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
)

// Snapshot is a JSON-serializable machine inventory report.
type Snapshot struct {
	CPU          CPUInfo           `json:"cpu"`
	RAM          RAMInfo           `json:"ram"`
	Screen       ScreenInfo        `json:"screen"`
	Networks     []NetworkInfo     `json:"networks,omitempty"`
	BlockDevices []BlockDeviceInfo `json:"blockDevices,omitempty"`
	TPM          TPMInfo           `json:"tpm"`
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
	Name  string `json:"name"`
	Size  string `json:"size,omitempty"`
	Model string `json:"model,omitempty"`
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
			}
			snap.BlockDevices = append(snap.BlockDevices, dev)
		}
	}

	return snap
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
