package desktoplayout_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStandaloneDesktopBuildLayout(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate desktop build-layout test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	tests := []struct {
		path     string
		contains []string
	}{
		{
			path:     "desktop/go.mod",
			contains: []string{"module github.com/DavidHoenisch/remotr/desktop"},
		},
		{
			path:     "desktop/wails.json",
			contains: []string{`"name": "remotr-desktop"`, `"outputfilename": "remotr-desktop"`},
		},
		{
			path: "desktop/main.go",
			contains: []string{
				"//go:embed all:frontend/dist",
				`"github.com/wailsapp/wails/v2"`,
				"func main()",
			},
		},
		{
			path:     "desktop/frontend/dist/index.html",
			contains: []string{"<html", `<div id="root"></div>`},
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, test.path))
			if err != nil {
				t.Fatalf("required standalone desktop artifact is absent: %v", err)
			}
			for _, fragment := range test.contains {
				if !strings.Contains(string(data), fragment) {
					t.Errorf("%s does not contain required build marker %q", test.path, fragment)
				}
			}
		})
	}
}
