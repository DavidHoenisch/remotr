package desktoplayout_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestRootModuleRemainsIndependentOfDesktopToolchain(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate desktop module-isolation test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	for _, relative := range []string{"go.mod", "vendor/modules.txt"} {
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read root dependency metadata %s: %v", relative, err)
		}
		if strings.Contains(string(content), "github.com/wailsapp/wails") {
			t.Errorf("root dependency metadata %s contains Wails", relative)
		}
	}

	skippedDirectories := map[string]bool{
		".agents": true,
		".git":    true,
		"desktop": true,
		"vendor":  true,
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == filepath.Join(root, "compose", "runtime") {
				return filepath.SkipDir
			}
			if path != root && skippedDirectories[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if importPath == "github.com/wailsapp/wails/v2" || strings.HasPrefix(importPath, "github.com/wailsapp/wails/v2/") {
				t.Errorf("root production file %s imports desktop-only package %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan root production imports: %v", err)
	}
}
