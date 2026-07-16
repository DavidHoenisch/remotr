package desktoplayout_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDesktopRepositoryBoundary(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate desktop repository-boundary test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	ignored := []string{
		"desktop/build/bin/remotr-desktop",
		"desktop/.cache/wails/state.json",
		"desktop/coverage.out",
		"desktop/frontend/node_modules/.modules.yaml",
		"desktop/frontend/dist/assets/index.js",
		"desktop/frontend/.vite/deps/react.js",
		"desktop/frontend/coverage/index.html",
		"desktop/frontend/playwright-report/index.html",
		"desktop/frontend/test-results/results.json",
		"desktop/frontend/tsconfig.tsbuildinfo",
	}
	retained := []string{
		"desktop/go.mod",
		"desktop/go.sum",
		"desktop/main.go",
		"desktop/wails.json",
		"desktop/frontend/package.json",
		"desktop/frontend/pnpm-lock.yaml",
		"desktop/frontend/index.html",
		"desktop/frontend/dist/.gitkeep",
		"desktop/frontend/dist/index.html",
		"desktop/frontend/src/App.tsx",
		"desktop/frontend/wailsjs/go/main/App.d.ts",
		"desktop/build/appicon.png",
		"desktop/build/linux/remotr-desktop.desktop",
	}

	for _, path := range ignored {
		t.Run("ignored/"+path, func(t *testing.T) {
			if !gitIgnores(t, root, path) {
				t.Errorf("%s is not ignored", path)
			}
		})
	}
	for _, path := range retained {
		t.Run("retained/"+path, func(t *testing.T) {
			if gitIgnores(t, root, path) {
				t.Errorf("%s is ignored", path)
			}
		})
	}
}

func gitIgnores(t *testing.T, root, path string) bool {
	t.Helper()
	command := exec.Command("git", "-C", root, "check-ignore", "--no-index", "--quiet", path)
	err := command.Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	t.Fatalf("inspect ignore decision for %s: %v", path, err)
	return false
}
