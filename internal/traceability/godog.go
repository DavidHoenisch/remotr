package traceability

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LintGodogFeatures requires every Gherkin scenario to declare at least one
// known, non-deferred OpenSpec tag directly above its Scenario heading.
func LintGodogFeatures(featuresDir string, manifest Manifest) ([]Issue, error) {
	var issues []Issue
	err := filepath.WalkDir(featuresDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".feature") {
			return nil
		}
		fileIssues, err := lintGodogFile(path, manifest)
		if err != nil {
			return err
		}
		issues = append(issues, fileIssues...)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return issues, err
}

func lintGodogFile(path string, manifest Manifest) ([]Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var issues []Issue
	var tags []string
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(text, "@") {
			tags = strings.Fields(text)
			continue
		}
		if strings.HasPrefix(text, "Scenario:") || strings.HasPrefix(text, "Scenario Outline:") {
			valid := false
			for _, tag := range tags {
				if !strings.HasPrefix(tag, "@os_") {
					continue
				}
				entry, exists := manifest.Scenarios[strings.TrimPrefix(tag, "@os_")]
				if exists && entry.Lifecycle != "deferred" && entry.Lifecycle != "not-applicable" && entry.Lifecycle != "removed" {
					valid = true
					break
				}
			}
			if !valid {
				issues = append(issues, Issue{fmt.Sprintf("%s:%d", path, line), "scenario requires a known active @os_<verification-id> tag"})
			}
			tags = nil
			continue
		}
		if text != "" && !strings.HasPrefix(text, "#") {
			tags = nil
		}
	}
	return issues, scanner.Err()
}
