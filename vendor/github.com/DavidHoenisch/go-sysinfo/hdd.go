package gosysinfo

import (
	"os"
	"path/filepath"
	"regexp"
)

const blockBase = "/sys/block"

type BlockDeviceInfo struct {
	Size  string
	Model string
}

type HDD interface {
	ListBlockDevices() ([]string, error)
	GetBlockDeviceInfo(devname string) (*BlockDeviceInfo, error)
}

var _ HDD = Reader{}

var partitionPattern = regexp.MustCompile(`^(.+)p[0-9]+$`)

func ListBlockDevices() ([]string, error) {
	entries, err := os.ReadDir(blockBase)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && !isPartition(name) {
			names = append(names, name)
		}
	}
	return names, nil
}

func GetBlockDeviceInfo(r SysReader, devname string) (*BlockDeviceInfo, error) {
	path, err := checkBlockDevicePath(devname)
	if err != nil {
		return nil, err
	}

	return &BlockDeviceInfo{
		Size:  readSysFile(r, filepath.Join(path, "size")),
		Model: readSysFile(r, filepath.Join(path, "device", "model")),
	}, nil
}

func (Reader) ListBlockDevices() ([]string, error) {
	return ListBlockDevices()
}

func (r Reader) GetBlockDeviceInfo(devname string) (*BlockDeviceInfo, error) {
	return GetBlockDeviceInfo(r, devname)
}

func checkBlockDevicePath(devname string) (string, error) {
	return checkBlockDevicePathFn(devname)
}

var checkBlockDevicePathFn = func(devname string) (string, error) {
	path := filepath.Join(blockBase, devname)
	if _, err := os.Stat(path); err != nil {
		return "", ErrBlockDeviceNotFound
	}
	return path, nil
}

func isPartition(name string) bool {
	return partitionPattern.MatchString(name)
}
