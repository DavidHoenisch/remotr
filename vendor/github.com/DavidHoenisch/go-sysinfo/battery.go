package gosysinfo

import (
	"os"
	"path/filepath"
	"strings"
)

const powerSupplyBase = "/sys/class/power_supply"

type BatteryInfo struct {
	Status                    string
	Capacity                  string
	CapacityLevel             string
	EnergyNow                 string
	EnergyFull                string
	EnergyFullDesign          string
	PowerNow                  string
	VoltageNow                string
	VoltageMinDesign          string
	Manufacturer              string
	ModelName                 string
	SerialNumber              string
	Technology                string
	Present                   string
	CycleCount                string
	ChargeControlEndThreshold string
}

type Battery interface {
	ListBatteries() ([]string, error)
	GetBatteryInfo(name string) (*BatteryInfo, error)
}

var _ Battery = Reader{}

func ListBatteries() ([]string, error) {
	names, err := listSysfsClassEntries(powerSupplyBase)
	if err != nil {
		return nil, err
	}
	return filterBatteryNames(names), nil
}

func filterBatteryNames(names []string) []string {
	batteries := make([]string, 0, len(names))
	for _, name := range names {
		if readPowerSupplyType(name) == "Battery" {
			batteries = append(batteries, name)
		}
	}
	return batteries
}

var readPowerSupplyType = func(name string) string {
	content, err := os.ReadFile(filepath.Join(powerSupplyBase, name, "type"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func GetBatteryInfo(r SysReader, name string) (*BatteryInfo, error) {
	path, err := checkBatteryPath(name)
	if err != nil {
		return nil, err
	}

	if readSysFile(r, filepath.Join(path, "type")) != "Battery" {
		return nil, ErrBatteryNotFound
	}

	read := func(field string) string {
		return readSysFile(r, filepath.Join(path, field))
	}

	return &BatteryInfo{
		Status:                    read("status"),
		Capacity:                  read("capacity"),
		CapacityLevel:             read("capacity_level"),
		EnergyNow:                 read("energy_now"),
		EnergyFull:                read("energy_full"),
		EnergyFullDesign:          read("energy_full_design"),
		PowerNow:                  read("power_now"),
		VoltageNow:                read("voltage_now"),
		VoltageMinDesign:          read("voltage_min_design"),
		Manufacturer:              read("manufacturer"),
		ModelName:                 read("model_name"),
		SerialNumber:              read("serial_number"),
		Technology:                read("technology"),
		Present:                   read("present"),
		CycleCount:                read("cycle_count"),
		ChargeControlEndThreshold: read("charge_control_end_threshold"),
	}, nil
}

func (Reader) ListBatteries() ([]string, error) {
	return ListBatteries()
}

func (r Reader) GetBatteryInfo(name string) (*BatteryInfo, error) {
	return GetBatteryInfo(r, name)
}

func checkBatteryPath(name string) (string, error) {
	return checkBatteryPathFn(name)
}

var checkBatteryPathFn = func(name string) (string, error) {
	path := filepath.Join(powerSupplyBase, name)
	if _, err := os.Stat(path); err != nil {
		return "", ErrBatteryNotFound
	}
	return path, nil
}
