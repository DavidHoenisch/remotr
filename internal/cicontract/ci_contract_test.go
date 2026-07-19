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
	workflow := string(workflowData)
	timeoutPattern := regexp.MustCompile(`(?m)^    timeout-minutes: ([0-9]+)$`)
	runPattern := regexp.MustCompile(`(?m)^          \./scripts/fuzz-all\.sh ([0-9]+)([sm])(?: \|.*)?$`)
	for _, campaign := range []struct {
		name string
		next string
	}{
		{name: "nightly", next: "\n  weekly:\n"},
		{name: "weekly"},
	} {
		marker := "  " + campaign.name + ":\n"
		start := strings.Index(workflow, marker)
		if start < 0 {
			t.Fatalf("scheduled fuzz workflow omits %s campaign", campaign.name)
		}
		section := workflow[start+len(marker):]
		if next := strings.Index(section, campaign.next); campaign.next != "" && next >= 0 {
			section = section[:next]
		}
		timeoutMatch := timeoutPattern.FindStringSubmatch(section)
		runMatch := runPattern.FindStringSubmatch(section)
		if timeoutMatch == nil || runMatch == nil {
			t.Fatalf("scheduled fuzz workflow has an unparseable %s campaign", campaign.name)
		}
		timeoutMinutes, _ := strconv.Atoi(timeoutMatch[1])
		duration, _ := strconv.Atoi(runMatch[1])
		if runMatch[2] == "m" {
			duration *= 60
		}
		campaignSeconds := targets * duration
		budgetSeconds := timeoutMinutes * 60
		if campaignSeconds > budgetSeconds {
			t.Errorf("%s fuzz campaign needs at least %ds for %d targets, exceeding its %ds timeout", campaign.name, campaignSeconds, targets, budgetSeconds)
		}
	}
}

func TestQualityGateResolvesComparisonBaseForManualDispatch(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "quality-gate.yml"))
	if err != nil {
		t.Fatalf("read quality workflow: %v", err)
	}
	workflow := string(data)
	for _, fragment := range []string{
		"name: Resolve comparison base",
		`base="$(git rev-parse HEAD^)"`,
		`echo "BASE_SHA=$base" >> "$GITHUB_ENV"`,
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("quality workflow does not contain manual-dispatch base resolution %q", fragment)
		}
	}
}

func TestQualityGateMutatesEveryChangedCriticalTargetWithVerifiedTool(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "quality-gate.yml"))
	if err != nil {
		t.Fatalf("read quality workflow: %v", err)
	}
	workflow := string(data)
	for _, fragment := range []string{
		"id: mutation_scope",
		`git diff --quiet "$base"...HEAD -- "$target"`,
		`MEWT=$(./scripts/install-mewt.sh "$RUNNER_TEMP/mewt-3.0.1")`,
		"MUTATION_TARGET_FILE: artifacts/mutation/changed-targets.txt",
		"make mutation-high-gate",
		"retention-days: 90",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("quality workflow omits changed-critical mutation contract %q", fragment)
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
