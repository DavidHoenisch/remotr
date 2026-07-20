package executil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
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

// UserProcess describes one shell-free command that must execute with an
// explicit unprivileged identity and bounded filesystem context.
type UserProcess struct {
	Name string
	Args []string
	Dir  string
	Home string
	UID  uint32
	GID  uint32
}

// UserRunner executes a command with the exact effective identity declared by
// UserProcess. It is used at provider boundaries that process untrusted input.
type UserRunner interface {
	Runner
	RunAsUser(context.Context, UserProcess) (stdout, stderr []byte, err error)
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

const maxSanitizedOutputBytes = 64 << 10

func (SanitizedOSRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- package providers supply argv
	cmd.Env = sanitizedEnvironment()
	stdout, stderr := newBoundedOutput(), newBoundedOutput()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (SanitizedOSRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- caller supplies argv
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = sanitizedEnvironment()
	stdout, stderr := newBoundedOutput(), newBoundedOutput()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// RunAsUser executes one command without a shell, with cleared supplementary
// groups and a fixed environment rooted in the supplied workspace.
func (SanitizedOSRunner) RunAsUser(ctx context.Context, process UserProcess) ([]byte, []byte, error) {
	if process.UID == 0 {
		return nil, nil, fmt.Errorf("executil: refusing privileged user process")
	}
	if process.Name == "" {
		return nil, nil, fmt.Errorf("executil: user process requires an executable")
	}
	if !cleanAbsolutePath(process.Dir) || !cleanAbsolutePath(process.Home) {
		return nil, nil, fmt.Errorf("executil: user process requires clean absolute directory and home paths")
	}
	cmd := exec.CommandContext(ctx, process.Name, process.Args...) // #nosec G204 -- package providers supply literal argv
	cmd.Dir = process.Dir
	cmd.Env = sanitizedUserEnvironment(process.Home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: process.UID, Gid: process.GID, Groups: []uint32{process.GID},
	}}
	stdout, stderr := newBoundedOutput(), newBoundedOutput()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func sanitizedEnvironment() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=/root", "DEBIAN_FRONTEND=noninteractive",
	}
}

func sanitizedUserEnvironment(home string) []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"GIT_TERMINAL_PROMPT=0", "DEBIAN_FRONTEND=noninteractive",
	}
}

func cleanAbsolutePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

type boundedOutput struct {
	buffer bytes.Buffer
}

func newBoundedOutput() boundedOutput { return boundedOutput{} }

func (b *boundedOutput) Write(value []byte) (int, error) {
	written := len(value)
	remaining := maxSanitizedOutputBytes - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buffer.Write(value)
	}
	return written, nil
}

func (b *boundedOutput) Bytes() []byte { return b.buffer.Bytes() }

// MockRunner records invocations and returns configured results.
type MockRunner struct {
	Calls     []MockCall
	UserCalls []UserProcess
	Inputs    []MockInput
	Next      map[string]MockResult
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

func (m *MockRunner) RunAsUser(_ context.Context, process UserProcess) ([]byte, []byte, error) {
	process.Args = append([]string(nil), process.Args...)
	m.UserCalls = append(m.UserCalls, process)
	if m.Next == nil {
		return nil, nil, fmt.Errorf("mock: no result for %s", m.key(process.Name, process.Args...))
	}
	r, ok := m.Next[m.key(process.Name, process.Args...)]
	if !ok {
		return nil, nil, fmt.Errorf("mock: no result for %s", m.key(process.Name, process.Args...))
	}
	return r.Stdout, r.Stderr, r.Err
}
