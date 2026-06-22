package inventory

import "strings"

type mapReader map[string]string

func (m mapReader) Read(path string) string {
	return strings.TrimSpace(m[path])
}
