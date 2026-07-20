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

func TestFuzzDiscoveryUsesExistingWorkingTreeTestFiles(t *testing.T) {
	root := repositoryRoot(t)
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts directory: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(root, "scripts", "fuzz-all.sh"))
	if err != nil {
		t.Fatalf("read fuzz discovery script: %v", err)
	}
	files := map[string]string{
		"go.mod": "module example.com/fuzz-discovery\n\ngo 1.26\n",
		"kept_test.go": `package fuzzdiscovery

import "testing"

` + `func FuzzKept(f *testing.F) {
	f.Add("seed")
	f.Fuzz(func(t *testing.T, input string) {})
}
`,
		"deleted_test.go": `package fuzzdiscovery

import "testing"

` + `func FuzzDeleted(f *testing.F) {
	f.Add("seed")
	f.Fuzz(func(t *testing.T, input string) {})
}
`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(repository, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	scriptPath := filepath.Join(repository, "scripts", "fuzz-all.sh")
	if err := os.WriteFile(scriptPath, script, 0o755); err != nil {
		t.Fatalf("write fuzz discovery script: %v", err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"add", "go.mod", "deleted_test.go", "scripts/fuzz-all.sh"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
		}
	}
	if err := os.Remove(filepath.Join(repository, "deleted_test.go")); err != nil {
		t.Fatalf("remove tracked fuzz test: %v", err)
	}

	command := exec.Command(scriptPath, "--seed-corpora", ".")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("discover working-tree fuzz targets with a deleted tracked test: %v\n%s", err, output)
	}
	got := string(output)
	if strings.Contains(got, "cannot open") {
		t.Fatalf("fuzz discovery tried to read a deleted tracked test:\n%s", got)
	}
	if !strings.Contains(got, "completed 1 discovered fuzz target(s) (seed corpora)") {
		t.Fatalf("fuzz discovery did not run only the remaining target:\n%s", got)
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
