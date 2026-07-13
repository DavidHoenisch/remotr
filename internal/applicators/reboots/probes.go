package reboots

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/executil"
)

type SystemProbes struct {
	Runner executil.Runner
}

func (p SystemProbes) BootID(context.Context) (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("read boot identity: %w", err)
	}
	bootID := strings.TrimSpace(string(raw))
	if bootID == "" {
		return "", fmt.Errorf("read boot identity: empty value")
	}
	return bootID, nil
}

func (p SystemProbes) OnACPower(context.Context) (bool, error) {
	types, err := filepath.Glob("/sys/class/power_supply/*/type")
	if err != nil {
		return false, fmt.Errorf("enumerate power supplies: %w", err)
	}
	foundBattery := false
	for _, typePath := range types {
		rawType, err := os.ReadFile(typePath)
		if err != nil {
			return false, fmt.Errorf("read power supply type: %w", err)
		}
		switch strings.TrimSpace(string(rawType)) {
		case "Battery":
			foundBattery = true
		case "Mains", "USB", "USB_C", "USB_PD":
			rawOnline, err := os.ReadFile(filepath.Join(filepath.Dir(typePath), "online"))
			if err != nil {
				return false, fmt.Errorf("read AC power state: %w", err)
			}
			if strings.TrimSpace(string(rawOnline)) == "1" {
				return true, nil
			}
		}
	}
	return !foundBattery, nil
}

func (p SystemProbes) ActiveUsers(context.Context) (bool, error) {
	stdout, _, err := p.runner().Run("loginctl", "list-users", "--no-legend", "--no-pager")
	if err != nil {
		return false, fmt.Errorf("list active users")
	}
	return strings.TrimSpace(string(stdout)) != "", nil
}

func (p SystemProbes) ActiveWorkloadInhibitors(context.Context) (bool, error) {
	stdout, _, err := p.runner().Run("systemd-inhibit", "--list", "--no-legend", "--no-pager", "--mode=block")
	if err != nil {
		return false, fmt.Errorf("list workload inhibitors")
	}
	return strings.TrimSpace(string(stdout)) != "", nil
}

func (p SystemProbes) runner() executil.Runner {
	if p.Runner == nil {
		return executil.OSRunner{}
	}
	return p.Runner
}
