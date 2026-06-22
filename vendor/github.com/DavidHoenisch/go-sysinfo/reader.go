package gosysinfo

import "os"

type SysReader interface {
	Read(string) string
}

var _ SysReader = Reader{}

type Reader struct{}

func (Reader) Read(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		// TODO: handle this error
	}

	return string(content)
}
