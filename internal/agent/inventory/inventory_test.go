package inventory

import (
	"encoding/json"
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
		"/proc/version":                         "Linux version 6.9.3-arch1-1 (linux@archlinux) (gcc (GCC) 14.1.1 20240522, GNU ld (GNU Binutils) 2.42.0) #1 SMP PREEMPT_DYNAMIC Fri, 31 May 2024 19:14:52 +0000",
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
