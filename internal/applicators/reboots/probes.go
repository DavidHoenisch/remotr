package reboots

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	active, err := shutdownBlockInhibitor(stdout)
	if err != nil {
		return false, fmt.Errorf("parse workload inhibitors")
	}
	return active, nil
}

func shutdownBlockInhibitor(output []byte) (bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		what, ok := inhibitorWhatField(fields)
		if !ok || fields[len(fields)-1] != "block" {
			return false, fmt.Errorf("unexpected inhibitor row")
		}
		if slicesContain(strings.Split(what, ":"), "shutdown") {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func inhibitorWhatField(fields []string) (string, bool) {
	for index := 0; index+5 < len(fields); index++ {
		if !unsignedDecimal(fields[index]) || !unsignedDecimal(fields[index+2]) {
			continue
		}
		what := fields[index+4]
		if validInhibitorOperations(what) {
			return what, true
		}
	}
	return "", false
}

func unsignedDecimal(value string) bool {
	_, err := strconv.ParseUint(value, 10, 32)
	return err == nil
}

func validInhibitorOperations(value string) bool {
	operations := strings.Split(value, ":")
	if len(operations) == 0 {
		return false
	}
	for _, operation := range operations {
		switch operation {
		case "shutdown", "sleep", "idle", "handle-power-key", "handle-reboot-key", "handle-suspend-key", "handle-hibernate-key", "handle-lid-switch":
		default:
			return false
		}
	}
	return true
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (p SystemProbes) runner() executil.Runner {
	if p.Runner == nil {
		return executil.OSRunner{}
	}
	return p.Runner
}
