package gosysinfo

import "os"

const (
	tpmBase               = "/sys/class/tpm/tpm0"
	tpmVersionMajor       = "/sys/class/tpm/tpm0/tpm_version_major"
	tpmDeviceDescription  = "/sys/class/tpm/tpm0/device/description"
)

type TPM interface {
	GetTpmVersion() string
	GetTpmDescription() string
}

var _ TPM = Reader{}

var tpmBaseExists = func() bool {
	_, err := os.Stat(tpmBase)
	return err == nil
}

func GetTpmVersion(r SysReader) string {
	if !tpmBaseExists() {
		return ""
	}
	return readSysFile(r, tpmVersionMajor)
}

func GetTpmDescription(r SysReader) string {
	if !tpmBaseExists() {
		return ""
	}
	return readSysFile(r, tpmDeviceDescription)
}

func (r Reader) GetTpmVersion() string {
	return GetTpmVersion(r)
}

func (r Reader) GetTpmDescription() string {
	return GetTpmDescription(r)
}
