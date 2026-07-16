package cicontract_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsTreeContainsNoBrokenSymlinks(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		if _, err := filepath.EvalSymlinks(path); err != nil {
			t.Errorf("docs symlink %s does not resolve: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs tree: %v", err)
	}
}

func TestFuzzDiscoveryDoesNotRequireRipgrep(t *testing.T) {
	root := repositoryRoot(t)
	path := t.TempDir()
	for _, name := range []string{"awk", "bash", "dirname", "git"} {
		target, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("locate %s: %v", name, err)
		}
		if err := os.Symlink(target, filepath.Join(path, name)); err != nil {
			t.Fatalf("expose %s in isolated PATH: %v", name, err)
		}
	}

	command := exec.Command(filepath.Join(root, "scripts", "fuzz-all.sh"), "1s", "./package-with-no-fuzz-target")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+path)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("fuzz discovery unexpectedly accepted a package with no fuzz target")
	}
	if got := string(output); !strings.Contains(got, "package selection matched no discovered native fuzz target") {
		t.Fatalf("fuzz discovery without rg failed before package selection:\n%s", got)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}
