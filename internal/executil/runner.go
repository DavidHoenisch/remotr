package executil

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Runner executes external commands (injectable for tests).
type Runner interface {
	Run(name string, args ...string) (stdout, stderr []byte, err error)
}

// InputRunner executes argv with protected stdin. Callers use it for secrets
// that must not appear in process arguments or structured diagnostics.
type InputRunner interface {
	Runner
	RunInput(name string, input []byte, args ...string) (stdout, stderr []byte, err error)
}

// OSRunner runs commands via os/exec.
type OSRunner struct{}

func (OSRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- caller supplies argv; used by applicators
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (OSRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- caller supplies argv
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// SanitizedOSRunner executes privileged provider commands with a fixed,
// noninteractive environment rather than inheriting endpoint/user secrets.
type SanitizedOSRunner struct{}

func (SanitizedOSRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- package providers supply argv
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=/root", "DEBIAN_FRONTEND=noninteractive",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (SanitizedOSRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- caller supplies argv
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=/root", "DEBIAN_FRONTEND=noninteractive",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// MockRunner records invocations and returns configured results.
type MockRunner struct {
	Calls  []MockCall
	Inputs []MockInput
	Next   map[string]MockResult
}

type MockCall struct {
	Name string
	Args []string
}

// MockInput records protected input separately from argv so tests can assert
// it never crosses the command-line boundary.
type MockInput struct {
	Name  string
	Args  []string
	Input []byte
}

type MockResult struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

func (m *MockRunner) key(name string, args ...string) string {
	return fmt.Sprintf("%s %v", name, args)
}

func (m *MockRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	m.Calls = append(m.Calls, MockCall{Name: name, Args: append([]string(nil), args...)})
	if m.Next == nil {
		return nil, nil, fmt.Errorf("mock: no result for %s", m.key(name, args...))
	}
	r, ok := m.Next[m.key(name, args...)]
	if !ok {
		return nil, nil, fmt.Errorf("mock: no result for %s", m.key(name, args...))
	}
	return r.Stdout, r.Stderr, r.Err
}

func (m *MockRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	m.Inputs = append(m.Inputs, MockInput{Name: name, Args: append([]string(nil), args...), Input: append([]byte(nil), input...)})
	return m.Run(name, args...)
}
