package firewalld

import (
	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/go-sysinfo/internal/probe"
)

type Reader struct {
	FS         gosysinfo.SysReader
	Cmd        probe.CommandRunner
	Privileged bool
}

func (r Reader) fs() gosysinfo.SysReader {
	if r.FS != nil {
		return r.FS
	}
	return gosysinfo.Reader{}
}

func (r Reader) cmd() probe.CommandRunner {
	if r.Cmd != nil {
		return r.Cmd
	}
	return probe.OSCommandRunner{}
}
