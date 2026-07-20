package executil_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
)

const maxProviderOutputBytes = 64 << 10

func TestSanitizedOSRunnerEnforcesProviderProcessBoundary(t *testing.T) {
	runner := executil.SanitizedOSRunner{}

	t.Run("literal argv and sanitized noninteractive environment", func(t *testing.T) {
		unexpectedPath := filepath.Join(t.TempDir(), "shell-was-executed")
		literal := "; touch " + unexpectedPath
		stdout, stderr, err := runner.Run(os.Args[0], "-test.run=^TestSanitizedOSRunnerHelper$", "--", "inspect", literal)
		if err != nil {
			t.Fatalf("Run() = %v, stderr=%q", err, stderr)
		}
		var observation struct {
			Args []string `json:"args"`
			Env  []string `json:"env"`
		}
		if err := json.NewDecoder(bytes.NewReader(stdout)).Decode(&observation); err != nil {
			t.Fatalf("decode helper output %q: %v", stdout, err)
		}
		if !slices.Equal(observation.Args, []string{literal}) {
			t.Fatalf("child argv = %#v, want one literal argument", observation.Args)
		}
		wantEnv := []string{
			"DEBIAN_FRONTEND=noninteractive",
			"HOME=/root",
			"LANG=C.UTF-8",
			"LC_ALL=C.UTF-8",
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		}
		slices.Sort(observation.Env)
		if !slices.Equal(observation.Env, wantEnv) {
			t.Fatalf("child environment = %#v, want %#v", observation.Env, wantEnv)
		}
		if _, err := os.Stat(unexpectedPath); !os.IsNotExist(err) {
			t.Fatalf("literal argv executed through a shell: stat error = %v", err)
		}
	})

	t.Run("stdout and stderr are bounded", func(t *testing.T) {
		stdout, stderr, err := runner.Run(os.Args[0], "-test.run=^TestSanitizedOSRunnerHelper$", "--", "flood")
		if err != nil {
			t.Fatalf("Run() = %v", err)
		}
		if len(stdout) != maxProviderOutputBytes || len(stderr) != maxProviderOutputBytes {
			t.Fatalf("captured output lengths = stdout:%d stderr:%d, want %d each", len(stdout), len(stderr), maxProviderOutputBytes)
		}
	})

	t.Run("protected stdin stays outside argv", func(t *testing.T) {
		const canary = "provider-stdin-secret-canary"
		stdout, stderr, err := runner.RunInput(os.Args[0], []byte(canary), "-test.run=^TestSanitizedOSRunnerHelper$", "--", "stdin")
		if err != nil {
			t.Fatalf("RunInput() = %v, stderr=%q", err, stderr)
		}
		got, _, _ := strings.Cut(string(stdout), "\n")
		if got != fmt.Sprintf("stdin-bytes=%d", len(canary)) {
			t.Fatalf("helper output = %q", got)
		}
		if strings.Contains(string(stdout), canary) || strings.Contains(string(stderr), canary) {
			t.Fatal("protected stdin was reflected into captured output")
		}
	})
}

func TestSanitizedOSRunnerHelper(t *testing.T) {
	marker := slices.Index(os.Args, "--")
	if marker < 0 || marker+1 >= len(os.Args) {
		t.Skip("subprocess helper")
	}
	switch os.Args[marker+1] {
	case "inspect":
		observation := struct {
			Args []string `json:"args"`
			Env  []string `json:"env"`
		}{Args: append([]string(nil), os.Args[marker+2:]...), Env: os.Environ()}
		if err := json.NewEncoder(os.Stdout).Encode(observation); err != nil {
			t.Fatal(err)
		}
	case "flood":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("o", maxProviderOutputBytes+4096))
		_, _ = io.WriteString(os.Stderr, strings.Repeat("e", maxProviderOutputBytes+4096))
	case "stdin":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "stdin-bytes=%d\n", len(input))
	default:
		t.Fatalf("unknown helper operation %q", os.Args[marker+1])
	}
}
