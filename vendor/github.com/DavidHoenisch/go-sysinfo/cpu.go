package gosysinfo

import (
	"strconv"
	"strings"
)

const procCPUInfo = "/proc/cpuinfo"

type CPUInfo struct {
	ModelName string
	VendorID  string
	CPUFamily string
	Model     string
	CoreCount string
}

type CPU interface {
	GetCPU() *CPUInfo
}

var _ CPU = Reader{}

func GetCPU(r SysReader) *CPUInfo {
	content := readSysFile(r, procCPUInfo)
	if content == "" {
		return &CPUInfo{}
	}

	info := &CPUInfo{}
	coreCount := 0

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
		case "processor":
			coreCount++
		case "model name":
			if info.ModelName == "" {
				info.ModelName = value
			}
		case "vendor_id":
			if info.VendorID == "" {
				info.VendorID = value
			}
		case "cpu family":
			if info.CPUFamily == "" {
				info.CPUFamily = value
			}
		case "model":
			if info.Model == "" {
				info.Model = value
			}
		}
	}

	if coreCount > 0 {
		info.CoreCount = strconv.Itoa(coreCount)
	}

	return info
}

func (r Reader) GetCPU() *CPUInfo {
	return GetCPU(r)
}
