package gosysinfo

import "strings"

func readSysFile(r SysReader, path string) string {
	return strings.TrimSpace(r.Read(path))
}
