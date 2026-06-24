package gosysinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	blockBase      = "/sys/block"
	blockClassBase = "/sys/class/block"
	udevDataBase   = "/run/udev/data"
)

type BlockDeviceInfo struct {
	Size           string
	Model          string
	Encrypted      bool
	EncryptionType string
}

type HDD interface {
	ListBlockDevices() ([]string, error)
	GetBlockDeviceInfo(devname string) (*BlockDeviceInfo, error)
}

var _ HDD = Reader{}

var partitionPattern = regexp.MustCompile(`^(.+)p[0-9]+$`)

func ListBlockDevices() ([]string, error) {
	names, err := listSysfsClassEntries(blockBase)
	if err != nil {
		return nil, err
	}

	devices := make([]string, 0, len(names))
	for _, name := range names {
		if !isPartition(name) {
			devices = append(devices, name)
		}
	}
	return devices, nil
}

func GetBlockDeviceInfo(r SysReader, devname string) (*BlockDeviceInfo, error) {
	path, err := checkBlockDevicePath(devname)
	if err != nil {
		return nil, err
	}

	encrypted, encryptionType := encryptionForDevice(r, devname, path)

	return &BlockDeviceInfo{
		Size:           readSysFile(r, filepath.Join(path, "size")),
		Model:          readSysFile(r, filepath.Join(path, "device", "model")),
		Encrypted:      encrypted,
		EncryptionType: encryptionType,
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
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	path = filepath.Join(blockClassBase, devname)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", ErrBlockDeviceNotFound
}

func isPartition(name string) bool {
	return partitionPattern.MatchString(name)
}

func encryptionForDevice(r SysReader, devname, path string) (bool, string) {
	if encrypted, encType := encryptionFromDMUUID(readSysFile(r, filepath.Join(path, "dm", "uuid"))); encrypted {
		return true, encType
	}
	if isPartition(devname) {
		return encryptionFromUdev(r, path)
	}
	return encryptionForDisk(r, devname, path)
}

func encryptionFromDMUUID(uuid string) (bool, string) {
	if !strings.HasPrefix(uuid, "CRYPT-") {
		return false, ""
	}
	rest := strings.TrimPrefix(uuid, "CRYPT-")
	end := strings.Index(rest, "-")
	if end <= 0 {
		return true, rest
	}
	return true, rest[:end]
}

func encryptionFromUdev(r SysReader, devPath string) (bool, string) {
	dev := readSysFile(r, filepath.Join(devPath, "dev"))
	if dev == "" {
		return false, ""
	}
	content := r.Read(filepath.Join(udevDataBase, "b"+dev))
	if content == "" {
		return false, ""
	}
	return encryptionTypeFromUdev(parseUdevProperties(content))
}

func encryptionTypeFromUdev(props map[string]string) (bool, string) {
	fsType := props["ID_FS_TYPE"]
	if fsType == "" || !strings.HasPrefix(fsType, "crypto") {
		return false, ""
	}
	switch props["ID_FS_VERSION"] {
	case "2":
		return true, "LUKS2"
	case "1":
		return true, "LUKS"
	default:
		return true, fsType
	}
}

func parseUdevProperties(content string) map[string]string {
	props := make(map[string]string)
	content = strings.ReplaceAll(content, "\x00", "\n")
	for _, field := range strings.Split(content, "\n") {
		if !strings.HasPrefix(field, "E:") {
			continue
		}
		field = strings.TrimPrefix(field, "E:")
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		props[key] = value
	}
	return props
}

var childPartitionsFn = childPartitions

func encryptionForDisk(r SysReader, diskName, diskPath string) (bool, string) {
	children, err := childPartitionsFn(diskPath)
	if err == nil {
		for _, child := range children {
			childPath, err := checkBlockDevicePath(child)
			if err != nil {
				continue
			}
			if encrypted, encType := encryptionFromUdev(r, childPath); encrypted {
				return true, encType
			}
		}
	}
	return encryptionFromDMSlaves(r, diskName)
}

func childPartitions(diskPath string) ([]string, error) {
	entries, err := os.ReadDir(diskPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		partPath := filepath.Join(diskPath, entry.Name(), "partition")
		if _, err := os.Stat(partPath); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func encryptionFromDMSlaves(r SysReader, diskName string) (bool, string) {
	names, err := listSysfsClassEntries(blockBase)
	if err != nil {
		return false, ""
	}

	for _, name := range names {
		if !strings.HasPrefix(name, "dm-") {
			continue
		}
		dmPath := filepath.Join(blockBase, name)
		slaves, err := listSysfsClassEntries(filepath.Join(dmPath, "slaves"))
		if err != nil {
			continue
		}
		for _, slave := range slaves {
			if strings.HasPrefix(slave, diskName) && slave != diskName {
				if encrypted, encType := encryptionFromDMUUID(readSysFile(r, filepath.Join(dmPath, "dm", "uuid"))); encrypted {
					return true, encType
				}
			}
		}
	}
	return false, ""
}
