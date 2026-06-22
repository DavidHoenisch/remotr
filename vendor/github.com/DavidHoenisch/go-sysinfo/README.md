# go-sysinfo

A small Go library for reading Linux system information from `/sys` and `/proc`. It exposes hardware and network facts through a consistent, testable API built around an injectable `SysReader`.

**Platform:** Linux only (reads from sysfs and procfs).

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

### HDD / block devices

List devices, then fetch details per device:

```go
devices, err := gosysinfo.ListBlockDevices()

info, err := gosysinfo.GetBlockDeviceInfo(r, "nvme0n1")
// info.Size, info.Model
```

`ListBlockDevices` skips partition nodes (for example `nvme0n1p1`).

### TPM

Two focused getters, gated on TPM presence:

```go
version := gosysinfo.GetTpmVersion(r)
desc := gosysinfo.GetTpmDescription(r)
```

Both return an empty string when no TPM is present at `/sys/class/tpm/tpm0`.

## Errors

| Error | When |
|-------|------|
| `ErrNetworkNotFound` | Unknown network interface |
| `ErrBlockDeviceNotFound` | Unknown block device name |

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

## License

No license file is included yet. All rights reserved unless otherwise specified by the repository owner.
