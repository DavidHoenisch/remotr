package nftables

import "github.com/DavidHoenisch/go-sysinfo/internal/probe"

type Reader struct {
	Cmd        probe.CommandRunner
	Privileged bool
}

func (r Reader) cmd() probe.CommandRunner {
	if r.Cmd != nil {
		return r.Cmd
	}
	return probe.OSCommandRunner{}
}
