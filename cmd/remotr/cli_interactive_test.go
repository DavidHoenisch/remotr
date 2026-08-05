package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
	"golang.org/x/sys/unix"
)

func TestHubSnippetPickerOptions_sortsFeaturedFirst(t *testing.T) {
	opts := hubSnippetPickerOptions([]hubcatalog.Entry{
		{ID: "b", Title: "Beta", Category: "manifests", SnippetPath: "snippets/b.yaml"},
		{ID: "a", Title: "Alpha", Category: "crons", SnippetPath: "snippets/a.yaml", Featured: true},
		{ID: "skip", Title: "Skip me", SnippetPath: ""},
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "a" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "Alpha  (crons · a)" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}

func TestEndpointPickerOptions_sortsAndLabelsFleet(t *testing.T) {
	opts := endpointPickerOptions([]admin.Endpoint{
		{ID: "laptop-b", Fleet: "platform", Usernames: []string{"bob"}},
		{ID: "laptop-a", Fleet: "engineering", Usernames: []string{"alice"}},
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "laptop-a" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "laptop-a  (engineering · alice)" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}

func TestAppPackagePickerOptions_sortsByNameAndVersion(t *testing.T) {
	opts := appPackagePickerOptions([]admin.AppPackage{
		{Name: "demo/cli", Version: "0.2.0"},
		{Name: "demo/cli", Version: "0.1.0"},
		{Name: "internal/mycli", Version: "1.0.0"},
	})
	if len(opts) != 3 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Key != "demo/cli@0.1.0" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
	name, version := parseAppPackagePickerValue(opts[2].Value)
	if name != "internal/mycli" || version != "1.0.0" {
		t.Fatalf("third value = %q %q", name, version)
	}
}

func TestEndpointLabelKeyOptions(t *testing.T) {
	opts := endpointLabelKeyOptions(map[string]string{
		"site": "berlin",
		"role": "web",
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "role" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "role=web" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}

func TestSecretPickerOptionsSortAndUseOnlySafeContext(t *testing.T) {
	opts := secretPickerOptions([]admin.LogicalSecretSummary{
		{Name: "wifi/office", Scope: "fleet", ActiveVersion: "2"},
		{Name: "ubuntu-pro/shared", Scope: "global"},
	})
	if len(opts) != 2 || opts[0].Value != "ubuntu-pro/shared" || opts[0].Key != "ubuntu-pro/shared  (global · inactive)" || opts[1].Key != "wifi/office  (fleet · active 2)" {
		t.Fatalf("secret picker options = %#v", opts)
	}
}

func TestSecretPickerSelectAndCancellationInPseudoTerminal(t *testing.T) {
	items := []admin.LogicalSecretSummary{{Name: "alpha/global", Scope: "global"}, {Name: "beta/fleet", Scope: "fleet", ActiveVersion: "1"}}
	t.Run("select", func(t *testing.T) {
		selected, err := runSecretPickerInPTY(t, items, "\r")
		if err != nil || selected != "alpha/global" {
			t.Fatalf("selected=%q err=%v", selected, err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		_, err := runSecretPickerInPTY(t, items, "\x03")
		if err == nil || strings.Contains(err.Error(), "timed out") {
			t.Fatalf("picker cancellation err=%v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, err := runSecretPickerInPTY(t, nil, "")
		if err == nil || !strings.Contains(err.Error(), "no secrets available") {
			t.Fatalf("empty picker err=%v", err)
		}
	})
}

func runSecretPickerInPTY(t *testing.T, items []admin.LogicalSecretSummary, input string) (string, error) {
	t.Helper()
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open pseudoterminal: %v", err)
	}
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("unlock pseudoterminal: %v", err)
	}
	ptyNumber, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("identify pseudoterminal: %v", err)
	}
	slaveFD, err := unix.Open("/dev/pts/"+strconv.Itoa(ptyNumber), unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatalf("open pseudoterminal slave: %v", err)
	}
	master := os.NewFile(uintptr(masterFD), fmt.Sprintf("ptmx-%d", ptyNumber))
	slave := os.NewFile(uintptr(slaveFD), fmt.Sprintf("pts-%d", ptyNumber))
	defer master.Close()
	resultPath := filepath.Join(t.TempDir(), "picker-result")
	command := exec.Command(os.Args[0], "-test.run=^TestSecretPickerPTYHelper$", "-test.count=1")
	command.Env = append(os.Environ(), "REMOTR_SECRET_PICKER_HELPER=1", "REMOTR_SECRET_PICKER_RESULT="+resultPath)
	if len(items) == 0 {
		command.Env = append(command.Env, "REMOTR_SECRET_PICKER_EMPTY=1")
	}
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := command.Start(); err != nil {
		_ = slave.Close()
		return "", err
	}
	_ = slave.Close()
	ready := make(chan struct{})
	go func() {
		first := make([]byte, 1)
		_, err := master.Read(first)
		close(ready)
		if err == nil {
			_, _ = io.Copy(io.Discard, master)
		}
	}()

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	if len(items) > 0 {
		select {
		case <-ready:
		case err := <-done:
			return "", err
		case <-time.After(time.Second):
			return "", fmt.Errorf("secret picker did not render")
		}
		<-time.After(25 * time.Millisecond)
		if _, err := master.Write([]byte(input)); err != nil {
			return "", err
		}
	}
	select {
	case processErr := <-done:
		raw, readErr := os.ReadFile(resultPath)
		if readErr != nil {
			if processErr != nil {
				return "", processErr
			}
			return "", readErr
		}
		result := string(raw)
		if strings.HasPrefix(result, "error:") {
			return "", fmt.Errorf("%s", strings.TrimPrefix(result, "error:"))
		}
		return result, processErr
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		return "", fmt.Errorf("secret picker timed out")
	}
}

func TestSecretPickerPTYHelper(t *testing.T) {
	if os.Getenv("REMOTR_SECRET_PICKER_HELPER") != "1" {
		return
	}
	items := []admin.LogicalSecretSummary{{Name: "alpha/global", Scope: "global"}, {Name: "beta/fleet", Scope: "fleet", ActiveVersion: "1"}}
	if os.Getenv("REMOTR_SECRET_PICKER_EMPTY") == "1" {
		items = nil
	}
	selected := ""
	err := promptSecretSelect(&selected, items)
	result := selected
	if err != nil {
		result = "error:" + err.Error()
	}
	if writeErr := os.WriteFile(os.Getenv("REMOTR_SECRET_PICKER_RESULT"), []byte(result), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
}
