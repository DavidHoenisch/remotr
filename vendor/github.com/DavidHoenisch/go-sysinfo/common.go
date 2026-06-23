package gosysinfo

import (
	"os"
	"path/filepath"
	"strings"
)

func readSysFile(r SysReader, path string) string {
	return strings.TrimSpace(r.Read(path))
}

func listSysfsClassEntries(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		info, err := os.Stat(filepath.Join(base, name))
		if err != nil || !info.IsDir() {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}
