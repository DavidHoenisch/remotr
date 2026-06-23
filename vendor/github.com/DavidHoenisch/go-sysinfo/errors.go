package gosysinfo

import "errors"

var (
	ErrNetworkNotFound     = errors.New("Network not found")
	ErrBlockDeviceNotFound = errors.New("Block device not found")
	ErrBatteryNotFound     = errors.New("Battery not found")
	ErrNotAvailable        = errors.New("integration not available")
)
