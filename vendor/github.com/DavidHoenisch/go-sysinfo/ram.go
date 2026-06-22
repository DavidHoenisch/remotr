package gosysinfo

import (
	"strings"
)

const procMemInfo = "/proc/meminfo"

type RAMInfo struct {
	MemTotal     string
	MemFree      string
	MemAvailable string
}

type RAM interface {
	GetRAM() *RAMInfo
}

var _ RAM = Reader{}

func GetRAM(r SysReader) *RAMInfo {
	content := readSysFile(r, procMemInfo)
	if content == "" {
		return &RAMInfo{}
	}

	info := &RAMInfo{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "MemTotal":
			info.MemTotal = value
		case "MemFree":
			info.MemFree = value
		case "MemAvailable":
			info.MemAvailable = value
		}
	}
	return info
}

func (r Reader) GetRAM() *RAMInfo {
	return GetRAM(r)
}
