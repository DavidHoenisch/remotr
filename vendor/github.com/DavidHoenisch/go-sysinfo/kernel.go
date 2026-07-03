package gosysinfo

import (
	"strings"
)

const procVersion = "/proc/version"

type KernelInfo struct {
	Version string
}

type Kernel interface {
	GetKernelVersion() *KernelInfo
}

var _ Kernel = Reader{}

func GetKernelVersion(r SysReader) *KernelInfo {
	content := readSysFile(r, procVersion)
	if content == "" {
		return &KernelInfo{}
	}

	// Expected format: Linux version 6.9.3-arch1-1 (linux@archlinux) ...
	const prefix = "Linux version "
	if strings.HasPrefix(content, prefix) {
		content = strings.TrimPrefix(content, prefix)
		if idx := strings.IndexByte(content, ' '); idx != -1 {
			return &KernelInfo{Version: content[:idx]}
		}
		return &KernelInfo{Version: content}
	}

	return &KernelInfo{}
}

func (r Reader) GetKernelVersion() *KernelInfo {
	return GetKernelVersion(r)
}
