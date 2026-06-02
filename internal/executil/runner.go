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

// MockRunner records invocations and returns configured results.
type MockRunner struct {
	Calls []MockCall
	Next  map[string]MockResult
}

type MockCall struct {
	Name string
	Args []string
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
