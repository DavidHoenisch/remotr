package gosysinfo

import (
	"strings"
)

const (
	fb0_virtual_size = "/sys/class/graphics/fb0/virtual_size"
	fb0_screen_modes = "/sys/class/graphics/fb0/modes"
)

type ScreenSize struct {
	X string
	Y string
}

type ScreenMode struct {
	Value string
}

type Graphics interface {
	GetScreenVirtualSize() ScreenSize
	GetScreenMode() ScreenMode
}

var _ Graphics = Reader{}

func GetScreenVirtualSize(r SysReader) ScreenSize {
	content := r.Read(fb0_virtual_size)
	if content == "" {
		return ScreenSize{X: "0", Y: "0"}
	}

	res := strings.Split(strings.TrimSpace(content), ",")
	if len(res) != 2 {
		return ScreenSize{X: "0", Y: "0"}
	}

	return ScreenSize{
		X: strings.TrimSpace(res[0]),
		Y: strings.TrimSpace(res[1]),
	}
}

func GetScreenMode(r SysReader) ScreenMode {
	content := r.Read(fb0_screen_modes)
	return ScreenMode{Value: strings.TrimSpace(content)}
}

func (r Reader) GetScreenVirtualSize() ScreenSize {
	return GetScreenVirtualSize(r)
}

func (r Reader) GetScreenMode() ScreenMode {
	return GetScreenMode(r)
}
