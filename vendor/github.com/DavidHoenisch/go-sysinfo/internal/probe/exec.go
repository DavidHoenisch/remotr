package probe

import (
	"fmt"
	"os/exec"
)

type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type MockCommandRunner map[string][]byte

func (m MockCommandRunner) Run(name string, args ...string) ([]byte, error) {
	key := commandKey(name, args...)
	if out, ok := m[key]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("command not mocked: %s", key)
}

func commandKey(name string, args ...string) string {
	key := name
	for _, arg := range args {
		key += "\x00" + arg
	}
	return key
}
