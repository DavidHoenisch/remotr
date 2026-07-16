package desktoplayout_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeLinuxSmokeGatesBuiltArtifactAdvertisement(t *testing.T) {
	root := repositoryRoot(t)

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}
	makefile := string(makefileData)
	for _, fragment := range []string{
		"DESKTOP_VERSION ?= dev",
		"desktop-smoke: desktop-build",
		`-ldflags "-X main.version=$(DESKTOP_VERSION)"`,
		`./scripts/desktop-native-smoke.sh --binary "$(DESKTOP_DIR)/build/bin/remotr-desktop" --version "$(DESKTOP_VERSION)"`,
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("root Makefile does not contain native smoke gate %q", fragment)
		}
	}

	workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "desktop.yml"))
	if err != nil {
		t.Fatalf("read desktop workflow: %v", err)
	}
	workflow := string(workflowData)
	for _, fragment := range []string{
		"xvfb xauth x11-utils",
		"name: Build and launch native Linux development snapshot",
		"DESKTOP_VERSION: 0.0.0-ci.${{ github.sha }}",
		"run: make desktop-smoke",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("desktop workflow does not contain native smoke contract %q", fragment)
		}
	}
	if strings.Contains(workflow, "actions/upload-artifact") {
		t.Error("desktop workflow advertises a Linux artifact before package-format evidence exists")
	}

	script := filepath.Join(root, "scripts", "desktop-native-smoke.sh")
	tests := []struct {
		name          string
		versionOutput string
		windowOutput  string
		launchExit    string
		wantErr       string
	}{
		{
			name:          "matching embedded version and native title",
			versionOutput: "Remotr Desktop v1.2.3",
			windowOutput:  `0x0400003 "Remotr Desktop": ("remotr-desktop" "remotr-desktop")`,
		},
		{
			name:          "wrong embedded version",
			versionOutput: "Remotr Desktop v9.9.9",
			windowOutput:  `0x0400003 "Remotr Desktop": ("remotr-desktop" "remotr-desktop")`,
			wantErr:       "embedded identity",
		},
		{
			name:          "wrong native title",
			versionOutput: "Remotr Desktop v1.2.3",
			windowOutput:  `0x0400003 "Other Application": ("remotr-desktop" "remotr-desktop")`,
			wantErr:       "Remotr Desktop window",
		},
		{
			name:          "process exits before window",
			versionOutput: "Remotr Desktop v1.2.3",
			launchExit:    "1",
			wantErr:       "exited before",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf 'Linux\\n'\n")
			writeExecutable(t, filepath.Join(bin, "xvfb-run"), `#!/bin/sh
if [ "${1-}" = "-a" ]; then
  shift
fi
exec "$@"
`)
			writeExecutable(t, filepath.Join(bin, "xwininfo"), "#!/bin/sh\nprintf '%s\\n' \"$FAKE_WINDOW_OUTPUT\"\n")
			binary := filepath.Join(bin, "remotr-desktop")
			writeExecutable(t, binary, `#!/bin/sh
if [ "${1-}" = "--version" ]; then
  printf '%s\n' "$FAKE_VERSION_OUTPUT"
  exit 0
fi
if [ "$FAKE_LAUNCH_EXIT" = "1" ]; then
  exit 23
fi
exec tail -f /dev/null
`)

			command := exec.Command(script, "--binary", binary, "--version", "v1.2.3")
			command.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_VERSION_OUTPUT="+test.versionOutput,
				"FAKE_WINDOW_OUTPUT="+test.windowOutput,
				"FAKE_LAUNCH_EXIT="+test.launchExit,
				"REMOTR_DESKTOP_SMOKE_ATTEMPTS=2",
				"REMOTR_DESKTOP_SMOKE_INTERVAL=0",
			)
			output, runErr := command.CombinedOutput()
			if test.wantErr == "" {
				if runErr != nil {
					t.Fatalf("native smoke failed: %v\n%s", runErr, output)
				}
				if !strings.Contains(string(output), "native launch smoke passed") {
					t.Errorf("native smoke output = %q, want passing evidence", output)
				}
				return
			}
			if runErr == nil {
				t.Fatalf("native smoke succeeded, want error containing %q", test.wantErr)
			}
			if !strings.Contains(string(output), test.wantErr) {
				t.Errorf("native smoke error = %q, want substring %q", output, test.wantErr)
			}
		})
	}
}
