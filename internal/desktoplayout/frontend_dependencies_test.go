package desktoplayout_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopFrontendDependenciesArePinnedAndRuntimeAssetsAreLocal(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate frontend dependency test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	frontend := filepath.Join(root, "desktop", "frontend")

	data, err := os.ReadFile(filepath.Join(frontend, "package.json"))
	if err != nil {
		t.Fatalf("read pinned frontend package manifest: %v", err)
	}
	var manifest struct {
		PackageManager  string            `json:"packageManager"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse frontend package manifest: %v", err)
	}
	if manifest.PackageManager != "pnpm@11.7.0" {
		t.Errorf("packageManager = %q, want exact pnpm@11.7.0", manifest.PackageManager)
	}

	required := []string{
		"@fontsource/ibm-plex-mono",
		"@fontsource/ibm-plex-sans",
		"@playwright/test",
		"@testing-library/jest-dom",
		"@testing-library/react",
		"@testing-library/user-event",
		"@types/react",
		"@types/react-dom",
		"@vitejs/plugin-react",
		"jsdom",
		"lucide-react",
		"react",
		"react-dom",
		"typescript",
		"vite",
		"vitest",
	}
	exactVersion := regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	for _, name := range required {
		version, ok := manifest.Dependencies[name]
		if !ok {
			version, ok = manifest.DevDependencies[name]
		}
		if !ok {
			t.Errorf("required frontend dependency %q is absent", name)
			continue
		}
		if !exactVersion.MatchString(version) {
			t.Errorf("dependency %s uses non-exact version %q", name, version)
		}
	}

	lockInfo, err := os.Stat(filepath.Join(frontend, "pnpm-lock.yaml"))
	if err != nil {
		t.Fatalf("inspect committed frontend lockfile: %v", err)
	}
	if lockInfo.Size() == 0 {
		t.Error("committed frontend lockfile is empty")
	}

	remoteAsset := regexp.MustCompile(`(?i)(?:src|href)\s*=\s*["'](?:https?:)?//|url\(\s*["']?(?:https?:)?//|["'](?:https?:)?//[a-z0-9]`)
	err = filepath.WalkDir(frontend, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".pnpm-store" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "package.json" || entry.Name() == "pnpm-lock.yaml" {
			return nil
		}
		switch filepath.Ext(path) {
		case ".css", ".html", ".js", ".jsx", ".mjs", ".svg", ".ts", ".tsx":
		default:
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if remoteAsset.Match(content) {
			t.Errorf("frontend runtime asset %s contains a remote URL", strings.TrimPrefix(path, root+string(filepath.Separator)))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan frontend runtime assets: %v", err)
	}
}
