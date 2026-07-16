package cicontract_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

func TestScheduledFuzzCampaignsFitTheirJobTimeouts(t *testing.T) {
	root := repositoryRoot(t)
	targetPattern := regexp.MustCompile(`(?m)^\s*func\s+Fuzz[A-Za-z0-9_]+\s*\(`)
	targets := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root {
				if entry.Name() == ".git" || entry.Name() == "vendor" {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		targets += len(targetPattern.FindAll(data, -1))
		return nil
	})
	if err != nil {
		t.Fatalf("discover native fuzz targets: %v", err)
	}
	if targets == 0 {
		t.Fatal("no native fuzz targets discovered")
	}

	workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "fuzz-campaigns.yml"))
	if err != nil {
		t.Fatalf("read scheduled fuzz workflow: %v", err)
	}
	campaignPattern := regexp.MustCompile(`(?ms)^  (nightly|weekly):\n.*?^    timeout-minutes: ([0-9]+)\n.*?^        run: ./scripts/fuzz-all.sh ([0-9]+)([sm])$`)
	matches := campaignPattern.FindAllStringSubmatch(string(workflowData), -1)
	if len(matches) != 2 {
		t.Fatalf("scheduled fuzz workflow defines %d parseable campaigns, want 2", len(matches))
	}
	for _, match := range matches {
		timeoutMinutes, _ := strconv.Atoi(match[2])
		duration, _ := strconv.Atoi(match[3])
		if match[4] == "m" {
			duration *= 60
		}
		campaignSeconds := targets * duration
		budgetSeconds := timeoutMinutes * 60
		if campaignSeconds > budgetSeconds {
			t.Errorf("%s fuzz campaign needs at least %ds for %d targets, exceeding its %ds timeout", match[1], campaignSeconds, targets, budgetSeconds)
		}
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
