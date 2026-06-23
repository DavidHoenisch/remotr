package gosysinfo

import (
	"strings"
)

const etcOSRelease = "/etc/os-release"

type OSReleaseInfo struct {
	Name              string
	PrettyName        string
	ID                string
	IDLike            string
	Version           string
	VersionID         string
	VersionCodename   string
	BuildID           string
	ANSIColor         string
	CPEName           string
	HomeURL           string
	DocumentationURL  string
	SupportURL        string
	BugReportURL      string
	PrivacyPolicyURL  string
	Variant           string
	VariantID         string
	Logo              string
}

type OSRelease interface {
	GetOSRelease() *OSReleaseInfo
}

var _ OSRelease = Reader{}

func GetOSRelease(r SysReader) *OSReleaseInfo {
	content := readSysFile(r, etcOSRelease)
	if content == "" {
		return &OSReleaseInfo{}
	}

	info := &OSReleaseInfo{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := unquoteOSReleaseValue(strings.TrimSpace(parts[1]))

		switch key {
		case "NAME":
			info.Name = value
		case "PRETTY_NAME":
			info.PrettyName = value
		case "ID":
			info.ID = value
		case "ID_LIKE":
			info.IDLike = value
		case "VERSION":
			info.Version = value
		case "VERSION_ID":
			info.VersionID = value
		case "VERSION_CODENAME":
			info.VersionCodename = value
		case "BUILD_ID":
			info.BuildID = value
		case "ANSI_COLOR":
			info.ANSIColor = value
		case "CPE_NAME":
			info.CPEName = value
		case "HOME_URL":
			info.HomeURL = value
		case "DOCUMENTATION_URL":
			info.DocumentationURL = value
		case "SUPPORT_URL":
			info.SupportURL = value
		case "BUG_REPORT_URL":
			info.BugReportURL = value
		case "PRIVACY_POLICY_URL":
			info.PrivacyPolicyURL = value
		case "VARIANT":
			info.Variant = value
		case "VARIANT_ID":
			info.VariantID = value
		case "LOGO":
			info.Logo = value
		}
	}

	return info
}

func unquoteOSReleaseValue(value string) string {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value
	}

	value = value[1 : len(value)-1]
	value = strings.ReplaceAll(value, `\"`, `"`)
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\\`, `\`)
	value = strings.ReplaceAll(value, `\$`, `$`)
	return value
}

func (r Reader) GetOSRelease() *OSReleaseInfo {
	return GetOSRelease(r)
}
