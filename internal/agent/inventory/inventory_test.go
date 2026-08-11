package inventory

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCollect(t *testing.T) {
	r := mapReader{
		"/etc/os-release": `NAME="Arch Linux"
PRETTY_NAME="Arch Linux"
ID=arch
VERSION_ID="rolling"
`,
		"/proc/cpuinfo":                        "processor\t: 0\nmodel name\t: Test CPU\nvendor_id\t: TestVendor\ncpu family\t: 6\nmodel\t\t: 42\nprocessor\t: 1\n",
		"/proc/meminfo":                        "MemTotal:       16384000 kB\nMemFree:         8192000 kB\nMemAvailable:   12288000 kB\n",
		"/proc/version":                        "Linux version 6.9.3-arch1-1 (linux@archlinux) (gcc (GCC) 14.1.1 20240522, GNU ld (GNU Binutils) 2.42.0) #1 SMP PREEMPT_DYNAMIC Fri, 31 May 2024 19:14:52 +0000",
		"/sys/class/graphics/fb0/virtual_size": "1920,1080",
		"/sys/class/graphics/fb0/modes":        "U:1920x1080p-0",
	}

	snap := Collect(r)

	if snap.OSRelease.PrettyName != "Arch Linux" {
		t.Fatalf("os pretty name = %q", snap.OSRelease.PrettyName)
	}
	if snap.CPU.ModelName != "Test CPU" {
		t.Fatalf("cpu model = %q", snap.CPU.ModelName)
	}
	if snap.CPU.CoreCount != "2" {
		t.Fatalf("cpu cores = %q", snap.CPU.CoreCount)
	}
	if snap.RAM.MemTotal != "16384000 kB" {
		t.Fatalf("ram total = %q", snap.RAM.MemTotal)
	}
	if snap.Screen.VirtualSize.X != "1920" || snap.Screen.VirtualSize.Y != "1080" {
		t.Fatalf("screen = %+v", snap.Screen)
	}
	if snap.Kernel.Version != "6.9.3-arch1-1" {
		t.Fatalf("kernel version = %q", snap.Kernel.Version)
	}

	raw, err := MarshalJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.CPU.ModelName != "Test CPU" {
		t.Fatalf("decoded cpu = %+v", decoded.CPU)
	}
	if decoded.OSRelease.ID != "arch" {
		t.Fatalf("decoded os = %+v", decoded.OSRelease)
	}
}

func TestDigest(t *testing.T) {
	snap := Snapshot{
		CPU: CPUInfo{ModelName: "Test CPU", CoreCount: "4"},
		RAM: RAMInfo{MemTotal: "8192 kB"},
	}
	d1, err := Digest(snap)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Digest(snap)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("digest not stable: %q vs %q", d1, d2)
	}
	if len(d1) != 64 {
		t.Fatalf("digest length = %d", len(d1))
	}

	snap.CPU.CoreCount = "8"
	d3, err := Digest(snap)
	if err != nil {
		t.Fatal(err)
	}
	if d3 == d1 {
		t.Fatal("digest should change when snapshot changes")
	}
}

func TestChangeDigestIgnoresRuntimeMeasurements(t *testing.T) {
	before := Snapshot{
		CPU: CPUInfo{ModelName: "Test CPU", CoreCount: "4"},
		RAM: RAMInfo{
			MemTotal: "16384000 kB", MemFree: "8192000 kB",
			MemAvailable: "12288000 kB",
		},
		Batteries: []BatteryInfo{{
			Name: "BAT0", Status: "Discharging", Capacity: "82",
			CapacityLevel: "Normal", PowerNow: "7420000",
			Technology: "Li-ion",
		}},
		Firewall: FirewallInfo{
			Backend: "nftables",
			Nftables: &NftablesInfo{
				RawRuleset: "tcp dport 443 counter packets 10 bytes 640 accept\n",
			},
		},
	}
	after := before
	after.RAM.MemFree = "4096000 kB"
	after.RAM.MemAvailable = "6144000 kB"
	after.Batteries = append([]BatteryInfo(nil), before.Batteries...)
	after.Batteries[0].Status = "Charging"
	after.Batteries[0].Capacity = "83"
	after.Batteries[0].CapacityLevel = "High"
	after.Batteries[0].PowerNow = "12500000"
	after.Firewall.Nftables = &NftablesInfo{
		RawRuleset: "tcp dport 443 counter packets 18 bytes 1152 accept\n",
	}

	beforePayload, err := Digest(before)
	if err != nil {
		t.Fatal(err)
	}
	afterPayload, err := Digest(after)
	if err != nil {
		t.Fatal(err)
	}
	if beforePayload == afterPayload {
		t.Fatal("complete payload digest ignored runtime measurements")
	}
	beforeChange, err := ChangeDigest(before)
	if err != nil {
		t.Fatal(err)
	}
	afterChange, err := ChangeDigest(after)
	if err != nil {
		t.Fatal(err)
	}
	if beforeChange != afterChange {
		t.Fatalf("change digest treated runtime measurements as inventory changes")
	}

	after.RAM.MemTotal = "32768000 kB"
	meaningfulChange, err := ChangeDigest(after)
	if err != nil {
		t.Fatal(err)
	}
	if meaningfulChange == beforeChange {
		t.Fatal("change digest ignored installed RAM change")
	}
	after.RAM.MemTotal = before.RAM.MemTotal
	after.Firewall.Nftables.RawRuleset =
		"tcp dport 8443 counter packets 18 bytes 1152 accept\n"
	meaningfulChange, err = ChangeDigest(after)
	if err != nil {
		t.Fatal(err)
	}
	if meaningfulChange == beforeChange {
		t.Fatal("change digest ignored firewall rule change")
	}
}

func BenchmarkChangeDigest400FirewallRules(b *testing.B) {
	snapshot := Snapshot{
		CPU: CPUInfo{ModelName: "Test CPU", CoreCount: "16"},
		RAM: RAMInfo{
			MemTotal: "65536000 kB", MemFree: "8192000 kB",
			MemAvailable: "12288000 kB",
		},
		Batteries: []BatteryInfo{{
			Name: "BAT0", Status: "Discharging", Capacity: "82",
			PowerNow: "7420000", Technology: "Li-ion",
		}},
		Firewall: FirewallInfo{
			Backend: "nftables",
			Nftables: &NftablesInfo{RawRuleset: strings.Repeat(
				"tcp dport 443 counter packets 10 bytes 640 accept\n",
				400,
			)},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := ChangeDigest(snapshot); err != nil {
			b.Fatal(err)
		}
	}
}
