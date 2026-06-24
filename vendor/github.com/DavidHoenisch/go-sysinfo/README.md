# go-sysinfo

A small Go library for reading Linux system information from `/sys`, `/proc`, and `/etc`. It exposes hardware, network, and OS facts through a consistent, testable API built around an injectable `SysReader`.

**Platform:** Linux only (reads from sysfs, procfs, and `/etc/os-release`).

## Install

```bash
go get github.com/DavidHoenisch/go-sysinfo
```

Requires Go 1.26 or later.

## Quick start

```go
package main

import (
	"fmt"
	"log"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
)

func main() {
	r := gosysinfo.Reader{}

	cpu := r.GetCPU()
	fmt.Printf("CPU: %s (%s cores)\n", cpu.ModelName, cpu.CoreCount)

	ram := r.GetRAM()
	fmt.Printf("RAM total: %s\n", ram.MemTotal)

	ifaces, err := r.ListNetworkInterfaces()
	if err != nil {
		log.Fatal(err)
	}
	for _, ifname := range ifaces {
		conn, err := r.GetNetworkConnectionInfo(ifname)
		if err != nil {
			continue
		}
		fmt.Printf("%s MAC: %s\n", ifname, conn.MACAddress)
	}
}
```

## API design

Every domain follows the same pattern:

- **Package functions** accept a `SysReader` for testability: `GetCPU(r SysReader)`.
- **`Reader` methods** delegate to those functions for production use: `Reader{}.GetCPU()`.
- **One getter per concern** — discovery, link-layer data, IP addresses, and statistics are separate calls rather than one large struct.
- **String values** are returned as read from the kernel (no unit conversion or numeric parsing).

### Core types

| Type | Role |
|------|------|
| `SysReader` | Interface with `Read(path string) string`; empty string means missing or unreadable |
| `Reader` | Production `SysReader` backed by `os.ReadFile` |

## Domains

### CPU

Parse `/proc/cpuinfo` into a single snapshot:

```go
info := gosysinfo.GetCPU(r)
// info.ModelName, info.VendorID, info.CPUFamily, info.Model, info.CoreCount
```

### RAM

Parse `/proc/meminfo`:

```go
info := gosysinfo.GetRAM(r)
// info.MemTotal, info.MemFree, info.MemAvailable
```

### Graphics

Read framebuffer info from `/sys/class/graphics/fb0/`:

```go
size := gosysinfo.GetScreenVirtualSize(r) // ScreenSize{X, Y}
mode := gosysinfo.GetScreenMode(r)        // ScreenMode{Value}
```

### Network

Discovery and detail are split:

```go
names, err := gosysinfo.ListNetworkInterfaces()

conn, err := gosysinfo.GetNetworkConnectionInfo(r, "eth0")
// conn.MACAddress from sysfs

ips, err := gosysinfo.GetNetworkIPInfo(r, "eth0")
// ips.IPv4 from /proc/net/fib_trie + /proc/net/route
// ips.IPv6 from /proc/net/if_inet6

stats, err := gosysinfo.GetNetworkStatistics(r, "eth0")
// per-field sysfs reads under /sys/class/net/<if>/
```

### Battery

List battery power supplies from `/sys/class/power_supply/`, then fetch details per battery:

```go
batteries, err := gosysinfo.ListBatteries()

info, err := gosysinfo.GetBatteryInfo(r, "BAT0")
// info.Status, info.Capacity, info.EnergyNow, info.PowerNow, info.Technology, ...
```

`ListBatteries` returns only entries whose `type` is `Battery` (AC adapters and other power supplies are skipped). Values are returned as read from sysfs (microjoules, microwatts, microvolts where applicable).

### HDD / block devices

List devices, then fetch details per device:

```go
devices, err := gosysinfo.ListBlockDevices()

info, err := gosysinfo.GetBlockDeviceInfo(r, "nvme0n1")
// info.Size, info.Model, info.Encrypted, info.EncryptionType
```

`ListBlockDevices` skips partition nodes (for example `nvme0n1p1`). You can still pass a partition name to `GetBlockDeviceInfo`; it resolves via `/sys/class/block/` when the device is not present under `/sys/block/`.

`Encrypted` and `EncryptionType` are detected from sysfs and udev without root:

- **Device-mapper volumes** (`dm-*`): reads `/sys/block/<name>/dm/uuid` for a `CRYPT-` prefix (for example `LUKS2`, `LUKS`, `PLAIN`).
- **Partitions** (for example `nvme0n1p2`): reads `/run/udev/data/b<major>:<minor>` for `ID_FS_TYPE=crypto_*` (LUKS containers).
- **Whole disks**: checks child partitions under the disk node and active dm slaves.

`EncryptionType` is empty when the device is not encrypted. Block-layer detection only; file/directory encryption (eCryptfs, fscrypt) is not covered.

### TPM

Two focused getters, gated on TPM presence:

```go
version := gosysinfo.GetTpmVersion(r)
desc := gosysinfo.GetTpmDescription(r)
```

Both return an empty string when no TPM is present at `/sys/class/tpm/tpm0`.

### OS release

Parse `/etc/os-release`:

```go
info := gosysinfo.GetOSRelease(r)
// info.Name, info.PrettyName, info.ID, info.VersionID, info.HomeURL, ...
```

Returns an empty struct when the file is missing or unreadable. Quoted values are unquoted per the os-release spec.

## Integrations

Optional subpackages read facts from installed software and desktop environments. They follow the same getter style as core but use injectable backends (unix sockets, commands, environment) instead of sysfs/proc alone.

The core package stays zero-dependency. Subpackages may import core for `SysReader`; core never imports subpackages.

```go
import (
	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/go-sysinfo/clamav"
	"github.com/DavidHoenisch/go-sysinfo/gnome"
	"github.com/DavidHoenisch/go-sysinfo/hyprland"
	"github.com/DavidHoenisch/go-sysinfo/omarchy"
)

cr := clamav.Reader{FS: gosysinfo.Reader{}}
if cr.Available() {
	fmt.Println(clamav.GetVersion(cr))
	fmt.Println(clamav.GetDatabaseStats(cr))
}

gr := gnome.Reader{FS: gosysinfo.Reader{}}
if gr.Available() {
	fmt.Println(gnome.GetSessionInfo(gr))
}

or := omarchy.Reader{}
if or.Available() {
	fmt.Println(omarchy.GetInfo(or))
}

hr := hyprland.Reader{}
if hr.Available() {
	fmt.Println(hyprland.GetSessionInfo(hr))
	fmt.Println(hyprland.OptionString(hyprland.GetOption(hr, "general:gaps_in")))
}
```

When an integration is not present, getters return empty values rather than errors. Use `ErrNotAvailable` only when you need to distinguish "integration absent" from "field missing."

### ClamAV (`go-sysinfo/clamav`)

Read-only clamd facts: availability, version, database stats, and config paths. Uses the clamd unix socket (`PING`, `VERSION`, `STATS`) with CLI fallback.

### GNOME (`go-sysinfo/gnome`)

Read-only GNOME session facts when GNOME is the active desktop: shell version, session env, and individual gsettings values.

### Omarchy (`go-sysinfo/omarchy`)

Read-only Omarchy facts via the `omarchy` CLI: version, branch, channel, current theme, font, and toggle state. Does not read config files directly.

### Hyprland (`go-sysinfo/hyprland`)

Read-only Hyprland runtime facts via `hyprctl -j`: version, monitors, workspaces, clients, keybinds, config errors, and effective config options (`getoption`). Works on any Hyprland install, not only Omarchy.

## Errors

| Error | When |
|-------|------|
| `ErrNetworkNotFound` | Unknown network interface |
| `ErrBlockDeviceNotFound` | Unknown block device name |
| `ErrBatteryNotFound` | Unknown or non-battery power supply name |
| `ErrNotAvailable` | Optional integration not present on the system |

Other failures (for example unreadable proc files) typically surface as empty strings rather than errors, matching the `SysReader` contract.

## Testing

Unit tests use a `mockReader` map that implements `SysReader`, so tests never touch the live filesystem:

```go
reader := mockReader{
	"/proc/cpuinfo": "processor\t: 0\nmodel name\t: Example CPU\n",
}
info := gosysinfo.GetCPU(reader)
```

Run tests:

```bash
go test ./...
```

Integration smoke tests (only on machines with clamd/GNOME installed):

```bash
go test -tags=integration ./clamav/... ./gnome/... ./omarchy/... ./hyprland/...
```

## License

No license file is included yet. All rights reserved unless otherwise specified by the repository owner.
